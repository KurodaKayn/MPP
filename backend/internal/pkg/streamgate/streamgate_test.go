package streamgate

import (
	"context"
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
