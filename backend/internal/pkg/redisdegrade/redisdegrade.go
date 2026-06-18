package redisdegrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

const (
	globalEnabledEnv          = "REDIS_DEGRADE_ENABLED"
	globalFailureThresholdEnv = "REDIS_DEGRADE_FAILURE_THRESHOLD"
	globalCoolDownEnv         = "REDIS_DEGRADE_COOLDOWN"

	defaultFailureThreshold = 3
	defaultCoolDown         = 30 * time.Second
)

type Group string

const (
	GroupDashboardProjectListCache  Group = "dashboard_project_list_cache"
	GroupDashboardContentSetupCache Group = "dashboard_content_setup_cache"
	GroupDashboardAccountCache      Group = "dashboard_account_cache"
	GroupDashboardStatsCache        Group = "dashboard_stats_cache"
	GroupResolvedMediaAssetCache    Group = "resolved_media_asset_cache"
	GroupRateLimit                  Group = "rate_limit"
)

var (
	metricsMu       sync.RWMutex
	metricsObserver MetricsObserver
)

// SetMetricsObserver registers a global MetricsObserver that is called
// after every operation attempted through a degrade guard. Pass nil to
// disable. The observer must be safe for concurrent use.
func SetMetricsObserver(obs MetricsObserver) {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	metricsObserver = obs
}

type stateCode int32

const (
	stateClosed   stateCode = 0
	stateOpen     stateCode = 1
	stateHalfOpen stateCode = 2
)

func stateToInt(s resilience.CircuitState) stateCode {
	switch s {
	case resilience.CircuitOpen:
		return stateOpen
	case resilience.CircuitHalfOpen:
		return stateHalfOpen
	default:
		return stateClosed
	}
}

type Guard struct {
	group     Group
	enabled   bool
	breaker   *resilience.CircuitBreaker
	lastState atomic.Int32
}

func NewGuard(group Group) *Guard {
	config := configFromEnv(group)
	guard := &Guard{
		group:   group,
		enabled: config.Enabled,
	}
	guard.lastState.Store(int32(stateClosed))
	if !config.Enabled {
		return guard
	}
	guard.breaker = resilience.NewCircuitBreaker("redis:"+string(group), config.FailureThreshold, config.CoolDown)
	return guard
}

func (g *Guard) Enabled() bool {
	return g != nil && g.enabled
}

func (g *Guard) State() resilience.CircuitState {
	if g == nil || g.breaker == nil {
		return resilience.CircuitClosed
	}
	return g.breaker.State()
}

func Do(guard *Guard, operation func() error) error {
	_, err := Call(guard, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

func Call[T any](guard *Guard, operation func() (T, error)) (T, error) {
	var zero T
	if operation == nil {
		return zero, errors.New("redis degrade operation is nil")
	}
	if guard == nil || !guard.Enabled() {
		return operation()
	}
	if err := guard.breaker.Allow(); err != nil {
		guard.emitMetrics("", true, err)
		return zero, fmt.Errorf("redis degraded for %s: %w", guard.group, err)
	}
	value, err := operation()
	guard.record(err)
	guard.emitMetrics("", false, err)
	return value, err
}

// CallWork is like Call but accepts a workload tag that is forwarded to
// the metrics observer, allowing callers to distinguish different
// workloads (e.g. "cache_read", "cache_write") within the same group.
func CallWork[T any](guard *Guard, workload string, operation func() (T, error)) (T, error) {
	var zero T
	if operation == nil {
		return zero, errors.New("redis degrade operation is nil")
	}
	if guard == nil || !guard.Enabled() {
		return operation()
	}
	if err := guard.breaker.Allow(); err != nil {
		guard.emitMetrics(workload, true, err)
		return zero, fmt.Errorf("redis degraded for %s: %w", guard.group, err)
	}
	value, err := operation()
	guard.record(err)
	guard.emitMetrics(workload, false, err)
	return value, err
}

// DoWork is like Do but accepts a workload tag for metrics.
func DoWork(guard *Guard, workload string, operation func() error) error {
	if operation == nil {
		return errors.New("redis degrade operation is nil")
	}
	_, err := CallWork(guard, workload, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

func ShouldDegrade(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, resilience.ErrCircuitOpen) {
		return true
	}
	return retryableRedisError(err)
}

func retryableRedisError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, redis.Nil) || errors.Is(err, resilience.ErrCircuitOpen) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	message := strings.ToLower(err.Error())
	retryableFragments := []string{
		"loading redis is loading the dataset in memory",
		"masterdown",
		"readonly",
		"timeout",
		"timed out",
		"pool timeout",
		"connection refused",
		"connection reset",
		"connection closed",
		"broken pipe",
		"no route to host",
		"network is unreachable",
		"eof",
		"i/o timeout",
	}
	for _, fragment := range retryableFragments {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func (g *Guard) record(err error) {
	if g == nil || g.breaker == nil {
		return
	}
	switch {
	case err == nil:
		g.breaker.Record(true)
	case errors.Is(err, redis.Nil):
		g.breaker.Record(true)
	case retryableRedisError(err):
		g.breaker.Record(false)
	default:
		g.breaker.Record(true)
	}
	g.checkStateTransition()
}

func (g *Guard) checkStateTransition() {
	if g == nil {
		return
	}
	metricsMu.RLock()
	obs := metricsObserver
	metricsMu.RUnlock()
	if obs == nil {
		return
	}
	current := g.State()
	cur := int32(stateToInt(current))
	for {
		prev := g.lastState.Load()
		if cur == prev {
			return
		}
		if g.lastState.CompareAndSwap(prev, cur) {
			obs.ObserveStateChange(g.group, string(current))
			return
		}
	}
}

func (g *Guard) emitMetrics(workload string, degraded bool, err error) {
	metricsMu.RLock()
	obs := metricsObserver
	metricsMu.RUnlock()
	if obs == nil {
		return
	}
	obs.ObserveOperation(g.group, workload, degraded, err)
}

type config struct {
	Enabled          bool
	FailureThreshold int
	CoolDown         time.Duration
}

func configFromEnv(group Group) config {
	groupEnvBase := envBase(group)
	return config{
		Enabled:          envutil.Bool(groupEnvBase+"_ENABLED", envutil.Bool(globalEnabledEnv, true)),
		FailureThreshold: intEnv(groupEnvBase+"_FAILURE_THRESHOLD", intEnv(globalFailureThresholdEnv, defaultFailureThreshold)),
		CoolDown:         envutil.Duration(groupEnvBase+"_COOLDOWN", envutil.Duration(globalCoolDownEnv, defaultCoolDown)),
	}
}

func envBase(group Group) string {
	name := strings.ToUpper(string(group))
	name = strings.NewReplacer("-", "_", ".", "_").Replace(name)
	return "REDIS_DEGRADE_" + name
}

func intEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
