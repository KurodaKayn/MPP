package redisclient

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestPoolConfigFromEnvUsesZeroValueDefaults(t *testing.T) {
	clearPoolEnv(t)

	config, err := poolConfigFromEnv()

	require.NoError(t, err)
	require.Zero(t, config.PoolSize)
	require.Zero(t, config.MinIdleConns)
	require.Zero(t, config.MaxIdleConns)
	require.Zero(t, config.ConnMaxIdleTime)
	require.Zero(t, config.ConnMaxLifetime)
}

func TestNewFromEnvAppliesPoolOverrides(t *testing.T) {
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
			clearPoolEnv(t)
			t.Setenv(tt.envName, tt.value)

			_, err := poolConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.envName)
		})
	}
}

func clearPoolEnv(t *testing.T) {
	t.Helper()

	t.Setenv(poolSizeEnv, "")
	t.Setenv(minIdleConnsEnv, "")
	t.Setenv(maxIdleConnsEnv, "")
	t.Setenv(connMaxIdleTimeEnv, "")
	t.Setenv(connMaxLifetimeEnv, "")
}
