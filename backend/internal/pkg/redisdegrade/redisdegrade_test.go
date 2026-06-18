package redisdegrade

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

// testObserver is a stub MetricsObserver for testing.
type testObserver struct {
	mu           sync.Mutex
	stateChanges []string
	observations []struct {
		group    Group
		workload string
		degraded bool
	}
}

func (o *testObserver) ObserveOperation(group Group, workload string, degraded bool, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observations = append(o.observations, struct {
		group    Group
		workload string
		degraded bool
	}{group, workload, degraded})
}

func (o *testObserver) ObserveStateChange(group Group, state string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stateChanges = append(o.stateChanges, state)
}

func TestGuardOpensAfterFailuresAndRecoversAfterCooldown(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "true")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_FAILURE_THRESHOLD", "2")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_COOLDOWN", "50ms")

	guard := NewGuard(GroupRateLimit)
	require.True(t, guard.Enabled())
	require.Equal(t, resilience.CircuitClosed, guard.State())

	_, err := Call(guard, func() (int, error) {
		return 0, context.DeadlineExceeded
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, resilience.CircuitClosed, guard.State())

	_, err = Call(guard, func() (int, error) {
		return 0, errors.New("connection refused")
	})
	require.Error(t, err)
	require.Equal(t, resilience.CircuitOpen, guard.State())

	_, err = Call(guard, func() (int, error) {
		return 1, nil
	})
	require.ErrorIs(t, err, resilience.ErrCircuitOpen)

	time.Sleep(60 * time.Millisecond)

	value, err := Call(guard, func() (int, error) {
		return 7, nil
	})
	require.NoError(t, err)
	require.Equal(t, 7, value)
	require.Equal(t, resilience.CircuitClosed, guard.State())
}

func TestShouldDegradeRecognizesRetryableRedisFailures(t *testing.T) {
	require.False(t, ShouldDegrade(nil))
	require.False(t, ShouldDegrade(redis.Nil))
	require.False(t, ShouldDegrade(context.Canceled))
	require.True(t, ShouldDegrade(context.DeadlineExceeded))
	require.True(t, ShouldDegrade(errors.New("connection refused")))
	require.True(t, ShouldDegrade(errors.New("LOADING Redis is loading the dataset in memory")))
	require.False(t, ShouldDegrade(errors.New("validation failed")))
}

func TestCallWorkGuardDisabledPassesThrough(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "false")
	guard := NewGuard(GroupRateLimit)
	require.False(t, guard.Enabled())

	val, err := CallWork(guard, "cache_read", func() (int, error) {
		return 42, nil
	})
	require.NoError(t, err)
	require.Equal(t, 42, val)
}

func TestCallWorkNilGuardPassesThrough(t *testing.T) {
	val, err := CallWork(nil, "cache_read", func() (int, error) {
		return 99, nil
	})
	require.NoError(t, err)
	require.Equal(t, 99, val)
}

func TestCallWorkNilOperationReturnsError(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "true")
	guard := NewGuard(GroupRateLimit)

	_, err := CallWork[struct{}](guard, "cache_write", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "operation is nil")
}

func TestCallWorkSuccessPassesThrough(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "true")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_FAILURE_THRESHOLD", "3")
	guard := NewGuard(GroupRateLimit)

	val, err := CallWork(guard, "cache_read", func() (string, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", val)
}

func TestCallWorkBreakerOpenReturnsDegraded(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "true")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_FAILURE_THRESHOLD", "1")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_COOLDOWN", "1h")
	guard := NewGuard(GroupRateLimit)

	// Trip the breaker
	_, err := Call(guard, func() (struct{}, error) {
		return struct{}{}, context.DeadlineExceeded
	})
	require.Error(t, err)
	require.Equal(t, resilience.CircuitOpen, guard.State())

	// CallWork should return degraded error
	_, err = CallWork(guard, "cache_read", func() (int, error) {
		t.Fatal("operation should not be called when breaker is open")
		return 0, nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis degraded for rate_limit")
}

func TestDoWorkSuccess(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "true")
	guard := NewGuard(GroupRateLimit)

	called := false
	err := DoWork(guard, "cache_write", func() error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
}

func TestDoWorkNilGuardPassesThrough(t *testing.T) {
	called := false
	err := DoWork(nil, "cache_write", func() error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
}

func TestCallWorkNilOperationReturnsError2(t *testing.T) {
	_, err := CallWork[struct{}](nil, "work", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "operation is nil")
}

func TestDoWorkNilOperationReturnsError(t *testing.T) {
	err := DoWork(nil, "work", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "operation is nil")
}

func TestGuardConcurrentStateTransition(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_ENABLED", "true")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_FAILURE_THRESHOLD", "1")
	t.Setenv("REDIS_DEGRADE_RATE_LIMIT_COOLDOWN", "50ms")

	guard := NewGuard(GroupRateLimit)
	require.True(t, guard.Enabled())

	obs := &testObserver{}
	SetMetricsObserver(obs)
	t.Cleanup(func() { SetMetricsObserver(nil) })

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, _ = Call(guard, func() (int, error) {
				return 0, context.DeadlineExceeded
			})
		})
	}
	wg.Wait()

	obs.mu.Lock()
	defer obs.mu.Unlock()
	require.Contains(t, obs.stateChanges, string(resilience.CircuitOpen))
}
