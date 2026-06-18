package redisdegrade

// MetricsObserver is an optional callback interface that redisdegrade
// calls after each operation to emit application-level Redis metrics.
// Implementations must be safe for concurrent use.
type MetricsObserver interface {
	// ObserveOperation records a single Redis command attempted through
	// the degrade guard. group is the redisdegrade.Group, workload is a
	// caller-supplied tag (e.g. "cache_read", "cache_write", "rate_limit").
	// degraded indicates the circuit breaker was open and the operation
	// was short-circuited. err is the final error returned to the caller.
	ObserveOperation(group Group, workload string, degraded bool, err error)

	// ObserveStateChange records a circuit-breaker state transition.
	ObserveStateChange(group Group, state string)
}
