package redisclient

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestPoolConfigFromEnvUsesZeroValueDefaults(t *testing.T) {
	clearRedisEnv(t)

	config, err := poolConfigFromEnv()

	require.NoError(t, err)
	require.Zero(t, config.PoolSize)
	require.Zero(t, config.MinIdleConns)
	require.Zero(t, config.MaxIdleConns)
	require.Zero(t, config.ConnMaxIdleTime)
	require.Zero(t, config.ConnMaxLifetime)
}

func TestNewFromEnvAppliesPoolOverrides(t *testing.T) {
	clearRedisEnv(t)
	redisServer := miniredis.RunT(t)
	t.Setenv(addrEnv, redisServer.Addr())
	t.Setenv(dbEnv, "2")
	t.Setenv(poolSizeEnv, "32")
	t.Setenv(minIdleConnsEnv, "4")
	t.Setenv(maxIdleConnsEnv, "16")
	t.Setenv(connMaxIdleTimeEnv, "45s")
	t.Setenv(connMaxLifetimeEnv, "30m")

	client, err := NewFromEnv(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	options := client.Options()
	require.Equal(t, redisServer.Addr(), options.Addr)
	require.Equal(t, 2, options.DB)
	require.Equal(t, 32, options.PoolSize)
	require.Equal(t, 4, options.MinIdleConns)
	require.Equal(t, 16, options.MaxIdleConns)
	require.Equal(t, 45*time.Second, options.ConnMaxIdleTime)
	require.Equal(t, 30*time.Minute, options.ConnMaxLifetime)
}

func TestConfigFromEnvKeepsRedisOptionalForDirectMode(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "invalid db", envName: dbEnv, value: "not-a-number"},
		{name: "invalid pool size", envName: poolSizeEnv, value: "not-a-number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRedisEnv(t)
			t.Setenv(tt.envName, tt.value)

			_, err := ConfigFromEnv()

			require.ErrorIs(t, err, ErrNotConfigured)
		})
	}
}

func TestConfigFromEnvBuildsSentinelEndpoint(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, endpointModeSentinel)
	t.Setenv(sentinelAddrsEnv, " redis-ha-sentinel:26379,redis-ha-sentinel-1:26379 ")
	t.Setenv(passwordEnv, "redis-secret")
	t.Setenv(dbEnv, "3")
	t.Setenv(tlsEnv, "true")
	t.Setenv(poolSizeEnv, "24")
	t.Setenv(minIdleConnsEnv, "3")
	t.Setenv(maxIdleConnsEnv, "9")

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, endpointModeSentinel, config.EndpointMode)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, config.SentinelAddrs)
	require.Equal(t, sentinelMasterDefault, config.SentinelMasterName)
	require.Equal(t, "redis-secret", config.Password)
	require.Equal(t, 3, config.DB)
	require.True(t, config.TLS)

	options := failoverOptions(config)
	require.Equal(t, sentinelMasterDefault, options.MasterName)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, options.SentinelAddrs)
	require.Equal(t, "redis-secret", options.Password)
	require.Equal(t, 3, options.DB)
	require.NotNil(t, options.TLSConfig)
	require.Equal(t, 24, options.PoolSize)
	require.Equal(t, 3, options.MinIdleConns)
	require.Equal(t, 9, options.MaxIdleConns)
}

func TestConfigFromEnvUsesSentinelMasterOverride(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, endpointModeSentinel)
	t.Setenv(sentinelAddrsEnv, "redis-ha-sentinel:26379")
	t.Setenv(sentinelMasterEnv, "custom-master")

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, "custom-master", config.SentinelMasterName)
}

func TestConfigFromEnvRejectsMissingSentinelAddrs(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, endpointModeSentinel)

	_, err := ConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), endpointModeEnv)
}

func TestConfigFromEnvRejectsUnknownEndpointMode(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, "cluster")

	_, err := ConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), endpointModeEnv)
}

func TestPoolConfigFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "negative pool size", envName: poolSizeEnv, value: "-1"},
		{name: "invalid min idle conns", envName: minIdleConnsEnv, value: "many"},
		{name: "negative max idle conns", envName: maxIdleConnsEnv, value: "-1"},
		{name: "pool size over int32", envName: poolSizeEnv, value: "2147483648"},
		{name: "invalid idle time", envName: connMaxIdleTimeEnv, value: "30"},
		{name: "zero idle time", envName: connMaxIdleTimeEnv, value: "0s"},
		{name: "negative lifetime", envName: connMaxLifetimeEnv, value: "-1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRedisEnv(t)
			t.Setenv(tt.envName, tt.value)

			_, err := poolConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.envName)
		})
	}
}

func clearRedisEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		endpointModeEnv,
		addrEnv,
		passwordEnv,
		dbEnv,
		tlsEnv,
		sentinelAddrsEnv,
		sentinelMasterEnv,
		poolSizeEnv,
		minIdleConnsEnv,
		maxIdleConnsEnv,
		connMaxIdleTimeEnv,
		connMaxLifetimeEnv,
	} {
		t.Setenv(name, "")
	}
}
