package redisclient

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

	directClient, ok := client.(*redis.Client)
	require.True(t, ok)
	options := directClient.Options()
	require.Equal(t, redisServer.Addr(), options.Addr)
	require.Equal(t, 2, options.DB)
	require.Equal(t, 32, options.PoolSize)
	require.Equal(t, 4, options.MinIdleConns)
	require.Equal(t, 16, options.MaxIdleConns)
	require.Equal(t, 45*time.Second, options.ConnMaxIdleTime)
	require.Equal(t, 30*time.Minute, options.ConnMaxLifetime)
	require.Equal(t, 1*time.Second, options.DialTimeout)
	require.Equal(t, 1*time.Second, options.ReadTimeout)
	require.Equal(t, 1*time.Second, options.WriteTimeout)
	require.Equal(t, 1500*time.Millisecond, options.PoolTimeout)
	require.Equal(t, 1, options.MaxRetries)
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

	options := failoverOptions(config, RoleDefault)
	require.Equal(t, sentinelMasterDefault, options.MasterName)
	require.Equal(t, []string{"redis-ha-sentinel:26379", "redis-ha-sentinel-1:26379"}, options.SentinelAddrs)
	require.Equal(t, "redis-secret", options.Password)
	require.Equal(t, 3, options.DB)
	require.NotNil(t, options.TLSConfig)
	require.Equal(t, 24, options.PoolSize)
	require.Equal(t, 3, options.MinIdleConns)
	require.Equal(t, 9, options.MaxIdleConns)
}

func TestConfigFromEnvBuildsClusterEndpoint(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, endpointModeCluster)
	t.Setenv(addrEnv, " redis-cluster-0:6379,redis-cluster-1:6379 ")
	t.Setenv(passwordEnv, "redis-secret")
	t.Setenv(tlsEnv, "true")
	t.Setenv(tlsCACertEnv, testRedisCACertPEM)
	t.Setenv(tlsServerNameEnv, "redis.internal.example")
	t.Setenv(poolSizeEnv, "24")
	t.Setenv(minIdleConnsEnv, "3")
	t.Setenv(maxIdleConnsEnv, "9")

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, endpointModeCluster, config.EndpointMode)
	require.Equal(t, []string{"redis-cluster-0:6379", "redis-cluster-1:6379"}, config.ClusterAddrs)
	require.Equal(t, "redis-secret", config.Password)
	require.Zero(t, config.DB)
	require.True(t, config.TLS)

	options := clusterOptions(config, RoleQueue)
	require.Equal(t, []string{"redis-cluster-0:6379", "redis-cluster-1:6379"}, options.Addrs)
	require.Equal(t, "redis-secret", options.Password)
	require.Equal(t, clusterMaxRedirects, options.MaxRedirects)
	require.Equal(t, clusterStateReloadInterval, options.ClusterStateReloadInterval)
	require.Equal(t, 2, options.MaxRetries)
	require.Equal(t, 50*time.Millisecond, options.MinRetryBackoff)
	require.Equal(t, 250*time.Millisecond, options.MaxRetryBackoff)
	require.Equal(t, 1*time.Second, options.DialTimeout)
	require.Equal(t, 2*time.Second, options.ReadTimeout)
	require.Equal(t, 2*time.Second, options.WriteTimeout)
	require.NotNil(t, options.TLSConfig)
	require.Equal(t, "redis.internal.example", options.TLSConfig.ServerName)
	require.Equal(t, 24, options.PoolSize)
	require.Equal(t, 3, options.MinIdleConns)
	require.Equal(t, 9, options.MaxIdleConns)

	client := newClient(config, RoleQueue)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	_, ok := client.(*redis.ClusterClient)
	require.True(t, ok)
}

func TestConfigFromEnvBuildsClusterEndpointFromURLs(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, endpointModeCluster)
	t.Setenv(addrEnv, "rediss://cluster-user:cluster-pass@redis-cluster-0:6380?addr=redis-cluster-1:6379")

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, []string{"redis-cluster-0:6380", "redis-cluster-1:6379"}, config.ClusterAddrs)
	require.True(t, config.TLS)
	require.Equal(t, "cluster-user", config.Username)
	require.Equal(t, "cluster-pass", config.Password)

	options := clusterOptions(config, RoleDefault)
	require.Equal(t, "cluster-user", options.Username)
	require.Equal(t, "cluster-pass", options.Password)
	require.NotNil(t, options.TLSConfig)
}

func TestConfigFromEnvRejectsConflictingClusterURLPasswords(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, endpointModeCluster)
	t.Setenv(addrEnv, "redis://:one@redis-cluster-0:6379,redis://:two@redis-cluster-1:6379")

	_, err := ConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), "conflicting passwords")
}

func TestConfigFromEnvBuildsTLSOptions(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(addrEnv, "redis.example.invalid:6379")
	t.Setenv(tlsEnv, "true")
	t.Setenv(tlsCACertEnv, testRedisCACertPEM)
	t.Setenv(tlsServerNameEnv, "redis.internal.example")

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	require.Equal(t, testRedisCACertPEM, config.TLSCACert)
	require.Equal(t, "redis.internal.example", config.TLSServerName)
	options := options(config, RoleDefault)
	require.NotNil(t, options.TLSConfig)
	require.Equal(t, uint16(tls.VersionTLS12), options.TLSConfig.MinVersion)
	require.Equal(t, "redis.internal.example", options.TLSConfig.ServerName)
	require.NotNil(t, options.TLSConfig.RootCAs)
	require.NotSame(t, options.TLSConfig, config.tlsConfig())
}

func TestConfigFromEnvBuildsTLSOptionsFromCAFile(t *testing.T) {
	clearRedisEnv(t)
	caFile := t.TempDir() + "/redis-ca.pem"
	require.NoError(t, os.WriteFile(caFile, []byte(testRedisCACertPEM), 0o600))
	t.Setenv(addrEnv, "redis.example.invalid:6379")
	t.Setenv(tlsEnv, "true")
	t.Setenv(tlsCAFileEnv, caFile)

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	options := options(config, RoleDefault)
	require.NotNil(t, options.TLSConfig)
	require.NotNil(t, options.TLSConfig.RootCAs)
}

func TestConfigFromEnvBuildsTLSOptionsWithClientCertificate(t *testing.T) {
	clearRedisEnv(t)
	certFile, keyFile := writeTestRedisClientCertificate(t)
	t.Setenv(addrEnv, "redis.example.invalid:6379")
	t.Setenv(tlsEnv, "true")
	t.Setenv(tlsCertFileEnv, certFile)
	t.Setenv(tlsKeyFileEnv, keyFile)

	config, err := ConfigFromEnv()
	require.NoError(t, err)

	options := options(config, RoleDefault)
	require.NotNil(t, options.TLSConfig)
	require.Len(t, options.TLSConfig.Certificates, 1)
}

func TestConfigFromEnvRejectsInvalidTLSCA(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "inline ca", envName: tlsCACertEnv, value: "not pem"},
		{name: "ca file", envName: tlsCAFileEnv, value: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRedisEnv(t)
			t.Setenv(addrEnv, "redis.example.invalid:6379")
			t.Setenv(tlsEnv, "true")
			value := tt.value
			if tt.envName == tlsCAFileEnv {
				caFile := t.TempDir() + "/redis-ca.pem"
				require.NoError(t, os.WriteFile(caFile, []byte("not pem"), 0o600))
				value = caFile
			}
			t.Setenv(tt.envName, value)

			_, err := ConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.envName)
		})
	}
}

func TestConfigFromEnvRejectsIncompleteTLSClientCertificate(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "missing key", env: map[string]string{tlsCertFileEnv: "/tmp/redis-client.crt"}},
		{name: "missing cert", env: map[string]string{tlsKeyFileEnv: "/tmp/redis-client.key"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRedisEnv(t)
			t.Setenv(addrEnv, "redis.example.invalid:6379")
			t.Setenv(tlsEnv, "true")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := ConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tlsCertFileEnv)
			require.Contains(t, err.Error(), tlsKeyFileEnv)
		})
	}
}

func TestRoleSettingsMatchExpectedBaselines(t *testing.T) {
	tests := []struct {
		role               Role
		dialTimeout        time.Duration
		readTimeout        time.Duration
		writeTimeout       time.Duration
		poolTimeout        time.Duration
		maxRetries         int
		minRetryBackoff    time.Duration
		maxRetryBackoff    time.Duration
		dialerRetries      int
		dialerRetryTimeout time.Duration
	}{
		{
			role:               RoleCoordination,
			dialTimeout:        500 * time.Millisecond,
			readTimeout:        500 * time.Millisecond,
			writeTimeout:       500 * time.Millisecond,
			poolTimeout:        750 * time.Millisecond,
			maxRetries:         -1,
			minRetryBackoff:    -1,
			maxRetryBackoff:    -1,
			dialerRetries:      1,
			dialerRetryTimeout: 50 * time.Millisecond,
		},
		{
			role:               RoleCache,
			dialTimeout:        750 * time.Millisecond,
			readTimeout:        750 * time.Millisecond,
			writeTimeout:       750 * time.Millisecond,
			poolTimeout:        1 * time.Second,
			maxRetries:         1,
			minRetryBackoff:    25 * time.Millisecond,
			maxRetryBackoff:    150 * time.Millisecond,
			dialerRetries:      2,
			dialerRetryTimeout: 75 * time.Millisecond,
		},
		{
			role:               RoleQueue,
			dialTimeout:        1 * time.Second,
			readTimeout:        2 * time.Second,
			writeTimeout:       2 * time.Second,
			poolTimeout:        2 * time.Second,
			maxRetries:         2,
			minRetryBackoff:    50 * time.Millisecond,
			maxRetryBackoff:    250 * time.Millisecond,
			dialerRetries:      3,
			dialerRetryTimeout: 100 * time.Millisecond,
		},
		{
			role:               RoleSessionContinuity,
			dialTimeout:        750 * time.Millisecond,
			readTimeout:        1 * time.Second,
			writeTimeout:       1 * time.Second,
			poolTimeout:        1250 * time.Millisecond,
			maxRetries:         1,
			minRetryBackoff:    25 * time.Millisecond,
			maxRetryBackoff:    150 * time.Millisecond,
			dialerRetries:      2,
			dialerRetryTimeout: 75 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			settings := roleSettings(tt.role)
			require.Equal(t, tt.dialTimeout, settings.DialTimeout)
			require.Equal(t, tt.readTimeout, settings.ReadTimeout)
			require.Equal(t, tt.writeTimeout, settings.WriteTimeout)
			require.Equal(t, tt.poolTimeout, settings.PoolTimeout)
			require.Equal(t, tt.maxRetries, settings.MaxRetries)
			require.Equal(t, tt.minRetryBackoff, settings.MinRetryBackoff)
			require.Equal(t, tt.maxRetryBackoff, settings.MaxRetryBackoff)
			require.Equal(t, tt.dialerRetries, settings.DialerRetries)
			require.Equal(t, tt.dialerRetryTimeout, settings.DialerRetryTimeout)
		})
	}
}

func TestNewClientSetFromEnvBuildsDistinctRoleClients(t *testing.T) {
	clearRedisEnv(t)
	redisServer := miniredis.RunT(t)
	t.Setenv(addrEnv, redisServer.Addr())

	clients, err := NewClientSetFromEnv(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, clients.Close())
	})

	require.NotNil(t, clients.Default)
	require.NotNil(t, clients.Coordination)
	require.NotNil(t, clients.Cache)
	require.NotNil(t, clients.Queue)
	require.NotNil(t, clients.Session)
	require.NotSame(t, clients.Default, clients.Coordination)
	require.NotSame(t, clients.Default, clients.Queue)
	coordinationClient, ok := clients.Coordination.(*redis.Client)
	require.True(t, ok)
	queueClient, ok := clients.Queue.(*redis.Client)
	require.True(t, ok)
	sessionClient, ok := clients.Session.(*redis.Client)
	require.True(t, ok)
	require.Equal(t, 500*time.Millisecond, coordinationClient.Options().DialTimeout)
	require.Equal(t, 2*time.Second, queueClient.Options().ReadTimeout)
	require.Equal(t, 1*time.Second, sessionClient.Options().ReadTimeout)
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

func TestConfigFromEnvRejectsClusterWithNonZeroDB(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, "cluster")
	t.Setenv(addrEnv, "redis-cluster-0:6379")
	t.Setenv(dbEnv, "1")

	_, err := ConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), dbEnv)
	require.Contains(t, err.Error(), endpointModeCluster)
}

func TestConfigFromEnvRejectsUnknownEndpointMode(t *testing.T) {
	clearRedisEnv(t)
	t.Setenv(endpointModeEnv, "unknown")

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
		tlsCACertEnv,
		tlsCAFileEnv,
		tlsCertFileEnv,
		tlsKeyFileEnv,
		tlsServerNameEnv,
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

func writeTestRedisClientCertificate(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "redis-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	keyDER := x509.MarshalPKCS1PrivateKey(privateKey)

	dir := t.TempDir()
	certFile := filepath.Join(dir, "redis-client.crt")
	keyFile := filepath.Join(dir, "redis-client.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
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
