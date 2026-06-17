package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedisConnectionConfigFromEnvUsesDirectEndpoint(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisAddrEnv, "redis:6379")
	t.Setenv(redisPasswordEnv, "redis-secret")
	t.Setenv(redisDBEnv, "2")
	t.Setenv(redisTLSEnv, "true")

	config, err := redisConnectionConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, redisEndpointModeDirect, config.EndpointMode)
	require.Equal(t, "redis:6379", config.Addr)
	require.Equal(t, "redis-secret", config.Password)
	require.Equal(t, 2, config.DB)
	require.True(t, config.TLS)

	options := redisOptions(config)
	require.Equal(t, "redis:6379", options.Addr)
	require.Equal(t, "redis-secret", options.Password)
	require.Equal(t, 2, options.DB)
	require.NotNil(t, options.TLSConfig)
}

func TestRedisConnectionConfigFromEnvUsesSentinelEndpoint(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisEndpointModeEnv, redisEndpointModeSentinel)
	t.Setenv(redisSentinelAddrsEnv, " redis-ha-sentinel:26379,redis-ha-sentinel-1:26379 ")
	t.Setenv(redisSentinelMasterEnv, "mpp-redis-ha")
	t.Setenv(redisPasswordEnv, "redis-secret")
	t.Setenv(redisDBEnv, "4")

	config, err := redisConnectionConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, redisEndpointModeSentinel, config.EndpointMode)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, config.SentinelAddrs)
	require.Equal(t, "mpp-redis-ha", config.SentinelMasterName)
	require.Equal(t, "redis-secret", config.Password)
	require.Equal(t, 4, config.DB)

	options := redisFailoverOptions(config)
	require.Equal(t, "mpp-redis-ha", options.MasterName)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, options.SentinelAddrs)
	require.Equal(t, "redis-secret", options.Password)
	require.Equal(t, 4, options.DB)
}

func TestRedisConnectionConfigFromEnvKeepsRedisOptionalForDirectMode(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisDBEnv, "not-a-number")

	_, err := redisConnectionConfigFromEnv()

	require.ErrorIs(t, err, errRedisNotConfigured)
}

func TestRedisConnectionConfigFromEnvRejectsIncompleteSentinelEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "missing sentinel addrs", envName: redisSentinelMasterEnv, value: "mpp-redis-ha"},
		{name: "missing sentinel master", envName: redisSentinelAddrsEnv, value: "redis-ha-sentinel:26379"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRedisEnv(t)
			t.Setenv(redisEndpointModeEnv, redisEndpointModeSentinel)
			t.Setenv(tt.envName, tt.value)

			_, err := redisConnectionConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), redisEndpointModeEnv)
		})
	}
}

func clearRedisEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		redisEndpointModeEnv,
		redisAddrEnv,
		redisPasswordEnv,
		redisDBEnv,
		redisTLSEnv,
		redisSentinelAddrsEnv,
		redisSentinelMasterEnv,
	} {
		t.Setenv(name, "")
	}
}
