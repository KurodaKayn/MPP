package observability

import "github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"

// SetupRedisMetrics creates a RedisMetricsObserver, registers its metrics
// with the Suite registry, and installs it as the global redisdegrade
// metrics observer. Call this once during service bootstrap after New().
func (s *Suite) SetupRedisMetrics() *RedisMetricsObserver {
	if s == nil {
		return nil
	}
	obs := newRedisMetricsObserver(s.serviceName)
	obs.RegisterWith(s.registry)
	redisdegrade.SetMetricsObserver(obs)
	s.redisObserver = obs
	return obs
}

// RedisMetricsObserver returns the observer previously installed by
// SetupRedisMetrics. Returns nil if not set up.
func (s *Suite) RedisMetricsObserver() *RedisMetricsObserver {
	if s == nil {
		return nil
	}
	return s.redisObserver
}
