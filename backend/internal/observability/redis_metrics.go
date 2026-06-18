package observability

import (
	"errors"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
)

// RedisMetricsObserver implements redisdegrade.MetricsObserver and records
// application-level Redis metrics into Prometheus counters and gauges.
type RedisMetricsObserver struct {
	serviceName string
	operations  *prometheus.CounterVec
	errors      *prometheus.CounterVec
	fallbacks   *prometheus.CounterVec
	cacheHits   *prometheus.CounterVec
	cacheMisses *prometheus.CounterVec
	breakerOpen *prometheus.CounterVec
	stateGauge  *prometheus.GaugeVec
}

// newRedisMetricsObserver creates the observer and registers its metrics.
// The caller must register the returned collector with the same registry
// used by the Suite.
func newRedisMetricsObserver(serviceName string) *RedisMetricsObserver {
	obs := &RedisMetricsObserver{
		serviceName: serviceName,

		operations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_redis_operations_total",
			Help: "Total Redis operations attempted through degrade guards.",
		}, []string{"service", "group", "workload", "status"}),

		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_redis_errors_total",
			Help: "Total Redis operations that returned a non-nil error.",
		}, []string{"service", "group", "workload", "error_class"}),

		fallbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_redis_fallback_total",
			Help: "Total Redis operations short-circuited by open circuit breaker.",
		}, []string{"service", "group", "workload"}),

		cacheHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_redis_cache_hits_total",
			Help: "Total cache reads that returned a valid cached value.",
		}, []string{"service", "group"}),

		cacheMisses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_redis_cache_misses_total",
			Help: "Total cache reads that returned redis.Nil or invalid data.",
		}, []string{"service", "group"}),

		breakerOpen: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_redis_breaker_transitions_total",
			Help: "Total circuit-breaker state transitions observed.",
		}, []string{"service", "group", "to_state"}),

		stateGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mpp_redis_breaker_state",
			Help: "Current circuit-breaker state per group (0=closed, 1=open, 2=half_open).",
		}, []string{"service", "group"}),
	}

	return obs
}

// RegisterWith registers all collector families with the given registry.
func (o *RedisMetricsObserver) RegisterWith(reg *prometheus.Registry) {
	reg.MustRegister(
		o.operations,
		o.errors,
		o.fallbacks,
		o.cacheHits,
		o.cacheMisses,
		o.breakerOpen,
		o.stateGauge,
	)
}

// ObserveOperation implements redisdegrade.MetricsObserver.
func (o *RedisMetricsObserver) ObserveOperation(group redisdegrade.Group, workload string, degraded bool, err error) {
	g := string(group)
	if g == "" {
		g = "default"
	}
	w := strings.TrimSpace(workload)
	if w == "" {
		w = "default"
	}
	svc := o.serviceName

	if degraded {
		o.fallbacks.WithLabelValues(svc, g, w).Inc()
		o.operations.WithLabelValues(svc, g, w, "degraded").Inc()
		return
	}

	if errors.Is(err, redis.Nil) {
		o.operations.WithLabelValues(svc, g, w, "ok").Inc()
		if isCacheReadWorkload(w) || isCacheMissWorkload(w) {
			o.cacheMisses.WithLabelValues(svc, g).Inc()
		}
		return
	}

	if err != nil {
		o.operations.WithLabelValues(svc, g, w, "error").Inc()
		errClass := classifyError(err)
		o.errors.WithLabelValues(svc, g, w, errClass).Inc()
		return
	}

	o.operations.WithLabelValues(svc, g, w, "ok").Inc()

	// Classify cache hit vs miss.
	// A cache hit is a successful cache read; writes and invalidations are
	// tracked as operations only.
	switch {
	case isCacheMissWorkload(w):
		o.cacheMisses.WithLabelValues(svc, g).Inc()
	case isCacheReadWorkload(w):
		o.cacheHits.WithLabelValues(svc, g).Inc()
	}
}

// ObserveStateChange implements redisdegrade.MetricsObserver.
func (o *RedisMetricsObserver) ObserveStateChange(group redisdegrade.Group, state string) {
	g := string(group)
	if g == "" {
		g = "default"
	}
	svc := o.serviceName

	o.breakerOpen.WithLabelValues(svc, g, state).Inc()

	switch state {
	case "open":
		o.stateGauge.WithLabelValues(svc, g).Set(1)
	case "half_open":
		o.stateGauge.WithLabelValues(svc, g).Set(2)
	default:
		o.stateGauge.WithLabelValues(svc, g).Set(0)
	}
}

func classifyError(err error) string {
	if err == nil {
		return "none"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe"):
		return "connection"
	case strings.Contains(msg, "circuit breaker open"):
		return "degraded"
	case strings.Contains(msg, "eof") || strings.Contains(msg, "i/o"):
		return "io"
	default:
		return "other"
	}
}

func isCacheMissWorkload(w string) bool {
	return strings.HasPrefix(w, "cache_") && strings.HasSuffix(w, "miss")
}

func isCacheReadWorkload(w string) bool {
	return w == "cache_read" || strings.HasPrefix(w, "cache_read_")
}
