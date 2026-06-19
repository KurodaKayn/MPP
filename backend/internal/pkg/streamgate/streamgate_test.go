package streamgate

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLimiterEnforcesUserConnectionLimit(t *testing.T) {
	limiter := New(nil, Config{
		Enabled: true,
		AI: Limits{
			User: 1,
			TTL:  time.Minute,
		},
	})
	userID := uuid.New()
	req := AcquireRequest{
		Kind:     KindAI,
		UserID:   userID,
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
		Resource: "content",
	}

	lease, err := limiter.Acquire(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, lease.ID)

	_, err = limiter.Acquire(context.Background(), req)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLimitExceeded)

	require.NoError(t, lease.Release(context.Background()))
	lease, err = limiter.Acquire(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, lease.Release(context.Background()))
}

func TestLimiterSeparatesStreamKinds(t *testing.T) {
	limiter := New(nil, Config{
		Enabled: true,
		AI: Limits{
			User: 1,
			TTL:  time.Minute,
		},
		Browser: Limits{
			User: 1,
			TTL:  time.Minute,
		},
	})
	userID := uuid.New()

	aiLease, err := limiter.Acquire(context.Background(), AcquireRequest{
		Kind:     KindAI,
		UserID:   userID,
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
	})
	require.NoError(t, err)
	defer func() { _ = aiLease.Release(context.Background()) }()

	browserLease, err := limiter.Acquire(context.Background(), AcquireRequest{
		Kind:     KindBrowser,
		UserID:   userID,
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
	})
	require.NoError(t, err)
	defer func() { _ = browserLease.Release(context.Background()) }()
}

func TestKeyPartFallsBackWhenSanitizedValueIsEmpty(t *testing.T) {
	require.Equal(t, "unknown", keyPart("@@@"))
	require.Equal(t, "unknown", keyPart(" --- "))
}

func TestNormalizeResourceFallsBackWhenEmpty(t *testing.T) {
	require.Equal(t, "unknown", normalizeResource(""))
	require.Equal(t, "unknown", normalizeResource("   "))
	require.Equal(t, "session-1", normalizeResource(" session-1 "))
}

func TestRedisReleaseKeepsStateWhenOwnerPayloadChanged(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	limiter := New(client, Config{
		Enabled: true,
		Prefix:  "mpp:test:stream",
		AI: Limits{
			User:   2,
			Tenant: 2,
			IP:     2,
			Global: 2,
			TTL:    time.Minute,
		},
	})
	req := AcquireRequest{
		Kind:     KindAI,
		UserID:   uuid.New(),
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
		Resource: "content",
	}

	lease, err := limiter.Acquire(context.Background(), req)
	require.NoError(t, err)

	keys := limiter.keys(req, lease.ID)
	require.NoError(t, client.Set(context.Background(), keys[0], "new-owner-payload", time.Minute).Err())

	require.NoError(t, lease.Release(context.Background()))

	require.Equal(t, "new-owner-payload", client.Get(context.Background(), keys[0]).Val())
	for _, key := range keys[1:] {
		require.Equal(t, int64(1), client.ZCard(context.Background(), key).Val())
	}
}

func TestRedisReleaseRemovesIndexEntriesWhenConnectionKeyMissing(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	limiter := New(client, Config{
		Enabled: true,
		Prefix:  "mpp:test:stream",
		AI: Limits{
			User:   1,
			Tenant: 1,
			IP:     1,
			Global: 1,
			TTL:    time.Minute,
		},
	})
	req := AcquireRequest{
		Kind:     KindAI,
		UserID:   uuid.New(),
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
		Resource: "content",
	}

	lease, err := limiter.Acquire(context.Background(), req)
	require.NoError(t, err)

	keys := limiter.keys(req, lease.ID)
	require.NoError(t, client.Del(context.Background(), keys[0]).Err())

	require.NoError(t, lease.Release(context.Background()))

	require.Equal(t, int64(0), client.Exists(context.Background(), keys[0]).Val())
	for _, key := range keys[1:] {
		require.Equal(t, int64(0), client.ZCard(context.Background(), key).Val())
	}
	lease, err = limiter.Acquire(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, lease.Release(context.Background()))
}

func TestRedisLimiterUsesClusterSafeCommandShape(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	client.AddHook(streamGateClusterGuardHook{})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	limiter := New(client, Config{
		Enabled: true,
		Prefix:  "mpp:test:stream",
		AI: Limits{
			User:   2,
			Tenant: 2,
			IP:     2,
			Global: 2,
			TTL:    time.Minute,
		},
	})

	lease, err := limiter.Acquire(context.Background(), AcquireRequest{
		Kind:     KindAI,
		UserID:   uuid.New(),
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
		Resource: "content",
	})
	require.NoError(t, err)
	require.NoError(t, lease.Release(context.Background()))
}

func TestRedisLimiterCleansPartialAcquireAfterContextCancel(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx, cancel := context.WithCancel(context.Background())
	hook := &streamGateCancelAfterFirstEvalHook{cancel: cancel}
	client.AddHook(hook)

	limiter := New(client, Config{
		Enabled: true,
		Prefix:  "mpp:test:stream",
		AI: Limits{
			User:   2,
			Tenant: 2,
			IP:     2,
			Global: 2,
			TTL:    time.Minute,
		},
	})
	req := AcquireRequest{
		Kind:     KindAI,
		UserID:   uuid.New(),
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
		Resource: "content",
	}

	lease, err := limiter.Acquire(ctx, req)
	require.Error(t, err)
	require.Nil(t, lease)
	require.True(t, hook.cancelled.Load())

	keys := limiter.keys(req, "")
	for _, key := range keys[1:] {
		require.Equal(t, int64(0), client.ZCard(context.Background(), key).Val(), key)
	}
}

type streamGateClusterGuardHook struct{}

func (streamGateClusterGuardHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (streamGateClusterGuardHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := validateStreamGateRedisCommand(cmd); err != nil {
			return err
		}
		return next(ctx, cmd)
	}
}

func (streamGateClusterGuardHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if len(cmds) > 1 && !streamGateClientMetadataPipeline(cmds) {
			return fmt.Errorf("pipeline used %d commands: %s", len(cmds), streamGateCommandNames(cmds))
		}
		for _, cmd := range cmds {
			if err := validateStreamGateRedisCommand(cmd); err != nil {
				return err
			}
		}
		return next(ctx, cmds)
	}
}

func validateStreamGateRedisCommand(cmd redis.Cmder) error {
	args := cmd.Args()
	switch strings.ToLower(cmd.Name()) {
	case "eval":
		keyCount, ok := streamGateCommandIntArg(args, 2)
		if !ok {
			return fmt.Errorf("unexpected eval key count argument: %#v", args)
		}
		if keyCount > 1 {
			return fmt.Errorf("eval used %d keys", keyCount)
		}
	case "del", "unlink":
		if len(args) > 2 {
			return fmt.Errorf("%s used %d keys", cmd.Name(), len(args)-1)
		}
	}
	return nil
}

func streamGateCommandIntArg(args []any, index int) (int, bool) {
	if len(args) <= index {
		return 0, false
	}
	switch value := args[index].(type) {
	case int:
		return value, true
	case int64:
		if value > math.MaxInt || value < math.MinInt {
			return 0, false
		}
		return int(value), true
	case uint64:
		if value > math.MaxInt {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}

func streamGateCommandNames(cmds []redis.Cmder) string {
	names := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		names = append(names, cmd.Name())
	}
	return strings.Join(names, ",")
}

func streamGateClientMetadataPipeline(cmds []redis.Cmder) bool {
	if len(cmds) == 0 {
		return false
	}
	for _, cmd := range cmds {
		if !strings.EqualFold(cmd.Name(), "client") {
			return false
		}
	}
	return true
}

type streamGateCancelAfterFirstEvalHook struct {
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

func (h *streamGateCancelAfterFirstEvalHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *streamGateCancelAfterFirstEvalHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err == nil && strings.EqualFold(cmd.Name(), "eval") && h.cancelled.CompareAndSwap(false, true) {
			h.cancel()
		}
		return err
	}
}

func (h *streamGateCancelAfterFirstEvalHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
