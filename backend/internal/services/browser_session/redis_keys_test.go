package browsersession

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
)

func TestBrowserSessionRedisKeysUseApprovedHashTags(t *testing.T) {
	sessionID := uuid.New()
	userID := uuid.New()

	require.Equal(t, "session:"+sessionID.String(), mustRedisTag(t, browserSessionKey(sessionID)))
	require.Equal(t, "user:"+userID.String(), mustRedisTag(t, browserSessionActiveKey(userID, "Douyin")))
	require.True(t, rediskey.ShareTag(
		browserSessionKey(sessionID),
		browserSessionStreamCurrentKey(sessionID),
		browserSessionStreamTokenKey(sessionID, "TOKEN-HASH"),
	))
	require.Equal(t, browserSessionStreamTokenPrefix+rediskey.Tag("session", sessionID.String())+":", browserSessionStreamTokenKeyPrefixFor(sessionID))
}

func TestRedisConcurrencyQuotaCleansPartialAcquireAfterContextCancel(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx, cancel := context.WithCancel(context.Background())
	hook := &browserSessionCancelAfterFirstEvalHook{cancel: cancel}
	client.AddHook(hook)

	svc := &BrowserSessionService{}
	svc.UseRedisCoordination(client)
	svc.UseQuotaConfig(BrowserSessionQuotaConfig{
		UserConcurrencyLimit:   2,
		TenantConcurrencyLimit: 2,
	})
	userID := uuid.New()
	sessionID := uuid.New()
	tenantID := "tenant-1"

	err := svc.acquireRedisConcurrencyQuota(ctx, userID, tenantID, sessionID, time.Now().Add(time.Minute))
	require.Error(t, err)
	require.True(t, hook.cancelled.Load())
	require.Equal(t, int64(0), client.ZCard(context.Background(), browserSessionQuotaUserKey(userID)).Val())
	require.Equal(t, int64(0), client.ZCard(context.Background(), browserSessionQuotaTenantKey(tenantID)).Val())
}

func mustRedisTag(t *testing.T, key string) string {
	t.Helper()

	tag, ok := rediskey.ExtractTag(key)
	require.True(t, ok, key)
	return tag
}

type browserSessionCancelAfterFirstEvalHook struct {
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

func (h *browserSessionCancelAfterFirstEvalHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *browserSessionCancelAfterFirstEvalHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err == nil && strings.EqualFold(cmd.Name(), "eval") && h.cancelled.CompareAndSwap(false, true) {
			h.cancel()
		}
		return err
	}
}

func (h *browserSessionCancelAfterFirstEvalHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
