package platformaccount

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestInvalidateDashboardAccountCacheIgnoresRequestCancellation(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, redisClient.Close())
	})

	workspaceID := uuid.New()
	cacheKey := dashboardAccountCacheKey(workspaceID, douyinPlatform)
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, `{"cached":true}`, time.Minute).Err())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(db)
	service.UseRedis(redisClient)

	service.InvalidateDashboardAccountCache(ctx, workspaceID, douyinPlatform)

	require.False(t, redisServer.Exists(cacheKey))
}
