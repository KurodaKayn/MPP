package streamgate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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
	require.True(t, errors.Is(err, ErrLimitExceeded))

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
	defer aiLease.Release(context.Background())

	browserLease, err := limiter.Acquire(context.Background(), AcquireRequest{
		Kind:     KindBrowser,
		UserID:   userID,
		TenantID: "tenant-1",
		IP:       "203.0.113.10",
	})
	require.NoError(t, err)
	defer browserLease.Release(context.Background())
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
