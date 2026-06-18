package observability

import (
	"errors"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
)

// saveAndRestoreObserver clears the global metricsObserver on test cleanup
// to prevent test pollution when running in parallel or sequential order.
func saveAndRestoreObserver(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		redisdegrade.SetMetricsObserver(nil)
	})
}

func TestSetupRedisMetricsRegistersMetricsAndObserver(t *testing.T) {
	saveAndRestoreObserver(t)
	suite := New("test-redis-redisdegrade")
	e := echo.New()
	suite.RegisterRoutes(e)
	obs := suite.SetupRedisMetrics()
	if obs == nil {
		t.Fatal("SetupRedisMetrics returned nil")
	}
	if suite.RedisMetricsObserver() != obs {
		t.Fatal("RedisMetricsObserver() does not match installed observer")
	}

	obs.ObserveOperation("dashboard_project_list_cache", "cache_read", false, nil)
	obs.ObserveOperation("dashboard_project_list_cache", "cache_read", false, redis.Nil)
	obs.ObserveOperation("dashboard_project_list_cache", "cache_read_miss", false, nil)
	obs.ObserveOperation("rate_limit", "", false, nil)
	obs.ObserveOperation("dashboard_account_cache", "cache_write", false, nil)
	obs.ObserveOperation("dashboard_account_cache", "cache_write", false, errors.New("timeout"))
	obs.ObserveOperation("dashboard_stats_cache", "cache_read", true, errors.New("redis degraded"))
	obs.ObserveStateChange("dashboard_project_list_cache", "open")
	obs.ObserveStateChange("dashboard_project_list_cache", "closed")

	metrics := scrapeMetrics(t, e)

	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`service="test-redis-redisdegrade"`,
		`group="dashboard_project_list_cache"`,
		`workload="cache_read"`,
		`status="ok"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`service="test-redis-redisdegrade"`,
		`group="rate_limit"`,
		`workload="default"`,
		`status="ok"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`group="dashboard_account_cache"`,
		`workload="cache_write"`,
		`status="ok"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`group="dashboard_account_cache"`,
		`status="error"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`group="dashboard_stats_cache"`,
		`status="degraded"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_cache_hits_total", []string{
		`group="dashboard_project_list_cache"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_cache_misses_total", []string{
		`group="dashboard_project_list_cache"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_errors_total", []string{
		`group="dashboard_account_cache"`,
		`error_class="timeout"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_fallback_total", []string{
		`group="dashboard_stats_cache"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_breaker_transitions_total", []string{
		`group="dashboard_project_list_cache"`,
		`to_state="open"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_breaker_transitions_total", []string{
		`group="dashboard_project_list_cache"`,
		`to_state="closed"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_breaker_state", []string{
		`group="dashboard_project_list_cache"`,
		"} 0",
	})
}

func TestCacheWriteDoesNotCountAsHit(t *testing.T) {
	saveAndRestoreObserver(t)
	suite := New("test-cache-write")
	obs := suite.SetupRedisMetrics()

	e := echo.New()
	suite.RegisterRoutes(e)

	obs.ObserveOperation("dashboard_account_cache", "cache_write", false, nil)

	metrics := scrapeMetrics(t, e)

	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`group="dashboard_account_cache"`,
		`workload="cache_write"`,
		`status="ok"`,
	})
	for line := range strings.SplitSeq(metrics, "\n") {
		if strings.HasPrefix(line, "mpp_redis_cache_hits_total") &&
			strings.Contains(line, `group="dashboard_account_cache"`) {
			t.Fatalf("cache_write should not increment cache hits, found line %q", line)
		}
	}
}

func TestRedisNilCacheReadCountsAsMissNotError(t *testing.T) {
	saveAndRestoreObserver(t)
	suite := New("test-redis-nil")
	obs := suite.SetupRedisMetrics()

	e := echo.New()
	suite.RegisterRoutes(e)

	obs.ObserveOperation("dashboard_project_list_cache", "cache_read", false, redis.Nil)

	metrics := scrapeMetrics(t, e)

	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`group="dashboard_project_list_cache"`,
		`workload="cache_read"`,
		`status="ok"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_cache_misses_total", []string{
		`group="dashboard_project_list_cache"`,
	})
	for line := range strings.SplitSeq(metrics, "\n") {
		if strings.HasPrefix(line, "mpp_redis_errors_total") &&
			strings.Contains(line, `group="dashboard_project_list_cache"`) {
			t.Fatalf("redis.Nil should not increment redis errors, found line %q", line)
		}
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect string
	}{
		{"nil", nil, "none"},
		{"timeout", errors.New("dial tcp: i/o timeout"), "timeout"},
		{"connection", errors.New("connection refused"), "connection"},
		{"degraded", errors.New("redis degraded for cache: circuit breaker open"), "degraded"},
		{"io", errors.New("unexpected EOF"), "io"},
		{"other", errors.New("permission denied"), "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.err)
			if got != tt.expect {
				t.Errorf("classifyError(%v) = %q, want %q", tt.err, got, tt.expect)
			}
		})
	}
}

func TestRedisMetricsObserverNilSafe(t *testing.T) {
	obs := newRedisMetricsObserver("nil-test")
	obs.ObserveOperation("group", "", false, nil)
	obs.ObserveOperation("", "workload", false, nil)
	obs.ObserveOperation("", "", false, nil)
	obs.ObserveStateChange("", "open")
	obs.ObserveStateChange("group", "closed")
}

func TestRedisMetricsObserverInterfaceSatisfaction(t *testing.T) {
	var _ redisdegrade.MetricsObserver = (*RedisMetricsObserver)(nil)
}

func TestSetupRedisMetricsNilSuite(t *testing.T) {
	var s *Suite
	obs := s.SetupRedisMetrics()
	if obs != nil {
		t.Fatal("nil Suite.SetupRedisMetrics should return nil")
	}
	if s.RedisMetricsObserver() != nil {
		t.Fatal("nil Suite.RedisMetricsObserver should return nil")
	}
}

func TestRedisMetricsViaMetricsEndpoint(t *testing.T) {
	saveAndRestoreObserver(t)
	suite := New("test-redis-endpoint")
	suite.SetupRedisMetrics()

	e := echo.New()
	suite.RegisterRoutes(e)

	obs := suite.RedisMetricsObserver()
	obs.ObserveOperation("rate_limit", "", false, nil)
	obs.ObserveOperation("dashboard_project_list_cache", "cache_read", false, errors.New("timeout"))
	obs.ObserveOperation("dashboard_account_cache", "cache_read", true, errors.New("degraded"))

	metrics := scrapeMetrics(t, e)

	assertLineExists := func(metric string, labels []string) {
		t.Helper()
		for line := range strings.SplitSeq(metrics, "\n") {
			if !strings.HasPrefix(line, metric) {
				continue
			}
			matched := true
			for _, label := range labels {
				if !strings.Contains(line, label) {
					matched = false
					break
				}
			}
			if matched {
				return
			}
		}
		t.Fatalf("metric %s with labels %v not found in:\n%s", metric, labels, metrics)
	}

	assertLineExists("mpp_redis_operations_total", []string{`group="rate_limit"`, `status="ok"`})
	assertLineExists("mpp_redis_operations_total", []string{`group="dashboard_project_list_cache"`, `status="error"`})
	assertLineExists("mpp_redis_fallback_total", []string{`group="dashboard_account_cache"`})
	assertLineExists("mpp_redis_errors_total", []string{`group="dashboard_project_list_cache"`, `error_class="timeout"`})
}

func TestIsCacheMissWorkload(t *testing.T) {
	tests := []struct {
		input  string
		expect bool
	}{
		{"cache_read_miss", true},
		{"cache_miss", true},
		{"cache_write_miss", true},
		{"cache_read", false},
		{"cache_write", false},
		{"rate_limit", false},
		{"default", false},
		{"miss", false},
		{"redis_miss", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isCacheMissWorkload(tt.input)
			if got != tt.expect {
				t.Errorf("isCacheMissWorkload(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestIsCacheReadWorkload(t *testing.T) {
	tests := []struct {
		input  string
		expect bool
	}{
		{"cache_read", true},
		{"cache_read_miss", true},
		{"cache_write", false},
		{"cache_invalidate", false},
		{"rate_limit", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isCacheReadWorkload(tt.input)
			if got != tt.expect {
				t.Errorf("isCacheReadWorkload(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestCacheMissCountedCorrectly(t *testing.T) {
	saveAndRestoreObserver(t)
	suite := New("test-cache-miss")
	obs := suite.SetupRedisMetrics()

	e := echo.New()
	suite.RegisterRoutes(e)

	obs.ObserveOperation("dashboard_project_list_cache", "cache_read_miss", false, nil)

	metrics := scrapeMetrics(t, e)

	assertMetricLineContains(t, metrics, "mpp_redis_cache_misses_total", []string{
		`group="dashboard_project_list_cache"`,
	})
	assertMetricLineContains(t, metrics, "mpp_redis_operations_total", []string{
		`group="dashboard_project_list_cache"`,
		`workload="cache_read_miss"`,
		`status="ok"`,
	})
}
