package platformaccount

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestUseRedisStateStoreAndCacheWireIndependentClients(t *testing.T) {
	service := NewService(nil)
	stateClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	cacheClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})

	service.UseRedisStateStore(stateClient)
	service.UseRedisCache(cacheClient)

	require.Same(t, stateClient, service.stateStoreClient)
	require.Same(t, cacheClient, service.cache)
	require.IsType(t, &RedisXOAuth2StateStore{}, service.xOAuth2States)
}
