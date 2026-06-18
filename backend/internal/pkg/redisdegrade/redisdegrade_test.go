package redisdegrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

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
