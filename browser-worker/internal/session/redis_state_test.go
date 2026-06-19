package session

import (
	"crypto/tls"
	"os"
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
	require.Equal(t, redisDialTimeout, options.DialTimeout)
	require.Equal(t, redisReadTimeout, options.ReadTimeout)
	require.Equal(t, redisWriteTimeout, options.WriteTimeout)
	require.Equal(t, redisPoolTimeout, options.PoolTimeout)
	require.Equal(t, redisCommandRetries, options.MaxRetries)
	require.Equal(t, redisMinRetryBackoff, options.MinRetryBackoff)
	require.Equal(t, redisMaxRetryBackoff, options.MaxRetryBackoff)
	require.Equal(t, redisDialerRetries, options.DialerRetries)
	require.Equal(t, redisDialerRetryTimeout, options.DialerRetryTimeout)
}

func TestBrowserSessionRedisKeyUsesSessionHashTag(t *testing.T) {
	sessionID := "11111111-1111-4111-8111-111111111111"

	require.Equal(t, "mpp:browser:session:{session:"+sessionID+"}", browserSessionRedisKey(sessionID))
}

func TestRedisConnectionConfigFromEnvBuildsTLSOptions(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisAddrEnv, "redis.example.invalid:6379")
	t.Setenv(redisTLSEnv, "true")
	t.Setenv(redisTLSCACertEnv, testRedisCACertPEM)
	t.Setenv(redisTLSServerNameEnv, "redis.internal.example")

	config, err := redisConnectionConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, testRedisCACertPEM, config.TLSCACert)
	require.Equal(t, "redis.internal.example", config.TLSServerName)
	options := redisOptions(config)
	require.NotNil(t, options.TLSConfig)
	require.Equal(t, uint16(tls.VersionTLS12), options.TLSConfig.MinVersion)
	require.Equal(t, "redis.internal.example", options.TLSConfig.ServerName)
	require.NotNil(t, options.TLSConfig.RootCAs)
	require.NotSame(t, options.TLSConfig, config.tlsConfig())
}

func TestRedisConnectionConfigFromEnvBuildsTLSOptionsFromCAFile(t *testing.T) {
	clearRedisEnv(t)
	caFile := t.TempDir() + "/redis-ca.pem"
	require.NoError(t, os.WriteFile(caFile, []byte(testRedisCACertPEM), 0o600))
	t.Setenv(redisAddrEnv, "redis.example.invalid:6379")
	t.Setenv(redisTLSEnv, "true")
	t.Setenv(redisTLSCAFileEnv, caFile)

	config, err := redisConnectionConfigFromEnv()
	require.NoError(t, err)

	options := redisOptions(config)
	require.NotNil(t, options.TLSConfig)
	require.NotNil(t, options.TLSConfig.RootCAs)
}

func TestRedisConnectionConfigFromEnvRejectsInvalidTLSCA(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "inline ca", envName: redisTLSCACertEnv, value: "not pem"},
		{name: "ca file", envName: redisTLSCAFileEnv, value: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRedisEnv(t)
			t.Setenv(redisAddrEnv, "redis.example.invalid:6379")
			t.Setenv(redisTLSEnv, "true")
			value := tt.value
			if tt.envName == redisTLSCAFileEnv {
				caFile := t.TempDir() + "/redis-ca.pem"
				require.NoError(t, os.WriteFile(caFile, []byte("not pem"), 0o600))
				value = caFile
			}
			t.Setenv(tt.envName, value)

			_, err := redisConnectionConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.envName)
		})
	}
}

func TestRedisConnectionConfigFromEnvUsesSentinelEndpoint(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisEndpointModeEnv, redisEndpointModeSentinel)
	t.Setenv(redisSentinelAddrsEnv, " redis-ha-sentinel:26379,redis-ha-sentinel-1:26379 ")
	t.Setenv(redisPasswordEnv, "redis-secret")
	t.Setenv(redisDBEnv, "4")

	config, err := redisConnectionConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, redisEndpointModeSentinel, config.EndpointMode)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, config.SentinelAddrs)
	require.Equal(t, redisSentinelMasterDefault, config.SentinelMasterName)
	require.Equal(t, "redis-secret", config.Password)
	require.Equal(t, 4, config.DB)

	options := redisFailoverOptions(config)
	require.Equal(t, redisSentinelMasterDefault, options.MasterName)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, options.SentinelAddrs)
	require.Equal(t, "redis-secret", options.Password)
	require.Equal(t, 4, options.DB)
	require.Equal(t, redisDialTimeout, options.DialTimeout)
	require.Equal(t, redisReadTimeout, options.ReadTimeout)
	require.Equal(t, redisWriteTimeout, options.WriteTimeout)
	require.Equal(t, redisPoolTimeout, options.PoolTimeout)
	require.Equal(t, redisCommandRetries, options.MaxRetries)
	require.Equal(t, redisMinRetryBackoff, options.MinRetryBackoff)
	require.Equal(t, redisMaxRetryBackoff, options.MaxRetryBackoff)
	require.Equal(t, redisDialerRetries, options.DialerRetries)
	require.Equal(t, redisDialerRetryTimeout, options.DialerRetryTimeout)
}

func TestRedisConnectionConfigFromEnvKeepsRedisOptionalForDirectMode(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisDBEnv, "not-a-number")

	_, err := redisConnectionConfigFromEnv()

	require.ErrorIs(t, err, errRedisNotConfigured)
}

func TestRedisConnectionConfigFromEnvUsesSentinelMasterOverride(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisEndpointModeEnv, redisEndpointModeSentinel)
	t.Setenv(redisSentinelAddrsEnv, "redis-ha-sentinel:26379")
	t.Setenv(redisSentinelMasterEnv, "custom-master")

	config, err := redisConnectionConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, "custom-master", config.SentinelMasterName)
}

func TestRedisConnectionConfigFromEnvRejectsMissingSentinelAddrs(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(redisEndpointModeEnv, redisEndpointModeSentinel)

	_, err := redisConnectionConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), redisEndpointModeEnv)
}

func clearRedisEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		redisEndpointModeEnv,
		redisAddrEnv,
		redisPasswordEnv,
		redisDBEnv,
		redisTLSEnv,
		redisTLSCACertEnv,
		redisTLSCAFileEnv,
		redisTLSServerNameEnv,
		redisSentinelAddrsEnv,
		redisSentinelMasterEnv,
	} {
		t.Setenv(name, "")
	}
}

const testRedisCACertPEM = `-----BEGIN CERTIFICATE-----
MIIDEzCCAfugAwIBAgIUb15xgBiiAVRKRFX/A/p9TvypqJwwDQYJKoZIhvcNAQEL
BQAwGTEXMBUGA1UEAwwObXBwLXJlZGlzLXRlc3QwHhcNMjYwNjE4MTQyOTQ5WhcN
MjcwNjE4MTQyOTQ5WjAZMRcwFQYDVQQDDA5tcHAtcmVkaXMtdGVzdDCCASIwDQYJ
KoZIhvcNAQEBBQADggEPADCCAQoCggEBANGUc9qScjxCIirs4/uUnYWd+ikt1zJW
jhhbVGcDJe+Ooo1sB3MgUd1iEQMHhcYuYYA6qhircakcIF8kqx0gn29yWfPPA2uU
eKRMLZei7irkgM0ZoARM9WnHUsaPJ36sB3iEBGCC4OYUIFj9hBfIcUCzG/zU14qN
f0mXQLeLn8i3WtT9r47HJ30GcfE/upHO0Rd+GZPMmZbJ2y+oiH4Lrx8T+vL0U3SZ
XvTEPZmM0cYU5IQgLjqxkS0NrHzjPhP6+v75YZ354XJh0aLMAxIO+E1A8b7y457R
r4M0yBBvFOZqORR7zau0IMqq9dySm2FxOYv45R9gZMIzuEqOBvHg6xsCAwEAAaNT
MFEwHQYDVR0OBBYEFPKgwwRXzUJNYbeAxzEQaWvmjQXfMB8GA1UdIwQYMBaAFPKg
wwRXzUJNYbeAxzEQaWvmjQXfMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQEL
BQADggEBACX1ipm6cO0bgt3iB24CzFZ39ETCAs78UpQXll7VbkhPIJ9WoTYK11If
6mlhEOtDDcg1s1nY91wVmA5ZnLkAIY+RkBfIDREX9tzmhcROoJRJmu8LjTmW5QmF
KJV2w16drmHd7jgosOzFrqzWjatZ4DUyc9n8c4TYV0BDph6ARE0IL+9rHXA7wakG
tYGsODtHm/A35rOUUfx34E9PUIQXrm7HPIHbThi64/vJFd2dzvB/966Z2YCtkBf2
eXFaNn/Uv31V+R4jo/IoXT3Ge5aU2/HCF4GLt86Hny8lrZI/rzBtD+mvxHiPCeVH
kXlb94L5hmllJh6r7idCx5YrKWYGYCc=
-----END CERTIFICATE-----`
