package redisclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	endpointModeEnv    = "REDIS_ENDPOINT_MODE"
	addrEnv            = "REDIS_ADDR"
	passwordEnv        = "REDIS_PASSWORD"
	dbEnv              = "REDIS_DB"
	tlsEnv             = "REDIS_TLS"
	tlsCACertEnv       = "REDIS_TLS_CA_CERT"
	tlsCAFileEnv       = "REDIS_TLS_CA_FILE"
	tlsServerNameEnv   = "REDIS_TLS_SERVER_NAME"
	sentinelAddrsEnv   = "REDIS_SENTINEL_ADDRS"
	sentinelMasterEnv  = "REDIS_SENTINEL_MASTER_NAME"
	poolSizeEnv        = "REDIS_POOL_SIZE"
	minIdleConnsEnv    = "REDIS_MIN_IDLE_CONNS"
	maxIdleConnsEnv    = "REDIS_MAX_IDLE_CONNS"
	connMaxIdleTimeEnv = "REDIS_CONN_MAX_IDLE_TIME"
	connMaxLifetimeEnv = "REDIS_CONN_MAX_LIFETIME"
)

var ErrNotConfigured = errors.New("redis is not configured")

const (
	endpointModeDirect    = "direct"
	endpointModeSentinel  = "sentinel"
	sentinelMasterDefault = "mpp-redis-ha"
)

type Config struct {
	EndpointMode       string
	Addr               string
	Password           string
	DB                 int
	TLS                bool
	TLSCACert          string
	TLSCAFile          string
	TLSServerName      string
	SentinelAddrs      []string
	SentinelMasterName string
	tls                *tls.Config
	pool               poolConfig
}

type Role string

const (
	RoleDefault           Role = "default"
	RoleCoordination      Role = "coordination"
	RoleCache             Role = "cache"
	RoleQueue             Role = "queue"
	RoleSessionContinuity Role = "session_continuity"
)

type ClientSet struct {
	Default      *redis.Client
	Coordination *redis.Client
	Cache        *redis.Client
	Queue        *redis.Client
	Session      *redis.Client
}

type roleConfig struct {
	DialTimeout        time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	PoolTimeout        time.Duration
	MaxRetries         int
	MinRetryBackoff    time.Duration
	MaxRetryBackoff    time.Duration
	DialerRetries      int
	DialerRetryTimeout time.Duration
}

type poolConfig struct {
	PoolSize        int
	MinIdleConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

func NewFromEnv(ctx context.Context) (*redis.Client, error) {
	return NewRoleFromEnv(ctx, RoleDefault)
}

func NewRoleFromEnv(ctx context.Context, role Role) (*redis.Client, error) {
	config, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}

	return New(ctx, config, role)
}

func NewClientSetFromEnv(ctx context.Context) (*ClientSet, error) {
	config, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}

	clientSet := &ClientSet{}
	builders := []struct {
		role Role
		dst  **redis.Client
	}{
		{role: RoleDefault, dst: &clientSet.Default},
		{role: RoleCoordination, dst: &clientSet.Coordination},
		{role: RoleCache, dst: &clientSet.Cache},
		{role: RoleQueue, dst: &clientSet.Queue},
		{role: RoleSessionContinuity, dst: &clientSet.Session},
	}
	for _, builder := range builders {
		client, err := New(ctx, config, builder.role)
		if err != nil {
			_ = clientSet.Close()
			return nil, err
		}
		*builder.dst = client
	}
	return clientSet, nil
}

func ConfigFromEnv() (Config, error) {
	endpointMode, err := endpointModeFromEnv()
	if err != nil {
		return Config{}, err
	}

	config := Config{
		EndpointMode:  endpointMode,
		Addr:          strings.TrimSpace(os.Getenv(addrEnv)),
		Password:      strings.TrimSpace(os.Getenv(passwordEnv)),
		TLS:           envFlagEnabled(tlsEnv),
		TLSCACert:     strings.TrimSpace(os.Getenv(tlsCACertEnv)),
		TLSCAFile:     strings.TrimSpace(os.Getenv(tlsCAFileEnv)),
		TLSServerName: strings.TrimSpace(os.Getenv(tlsServerNameEnv)),
	}
	switch endpointMode {
	case endpointModeDirect:
		if config.Addr == "" {
			return Config{}, ErrNotConfigured
		}
	case endpointModeSentinel:
		config.SentinelAddrs = csvEnv(sentinelAddrsEnv)
		config.SentinelMasterName = strings.TrimSpace(os.Getenv(sentinelMasterEnv))
		if config.SentinelMasterName == "" {
			config.SentinelMasterName = sentinelMasterDefault
		}
		if len(config.SentinelAddrs) == 0 {
			return Config{}, fmt.Errorf("%s must be set when %s=sentinel", sentinelAddrsEnv, endpointModeEnv)
		}
	}

	db, err := redisDBFromEnv()
	if err != nil {
		return Config{}, err
	}
	pool, err := poolConfigFromEnv()
	if err != nil {
		return Config{}, err
	}
	config.DB = db
	config.pool = pool
	if err := config.configureTLS(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func New(ctx context.Context, config Config, role Role) (*redis.Client, error) {
	if err := config.configureTLS(); err != nil {
		return nil, err
	}
	client := newClient(config, role)
	if err := pingWithRetry(ctx, client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}

func newClient(config Config, role Role) *redis.Client {
	if config.EndpointMode == endpointModeSentinel {
		client := redis.NewFailoverClient(failoverOptions(config, role))
		client.AddHook(newLoggingHook(role))
		return client
	}
	client := redis.NewClient(options(config, role))
	client.AddHook(newLoggingHook(role))
	return client
}

func options(config Config, role Role) *redis.Options {
	roleSettings := roleSettings(role)
	options := &redis.Options{
		Addr:               config.Addr,
		Password:           config.Password,
		DB:                 config.DB,
		TLSConfig:          config.tlsConfig(),
		DialTimeout:        roleSettings.DialTimeout,
		ReadTimeout:        roleSettings.ReadTimeout,
		WriteTimeout:       roleSettings.WriteTimeout,
		PoolTimeout:        roleSettings.PoolTimeout,
		MaxRetries:         roleSettings.MaxRetries,
		MinRetryBackoff:    roleSettings.MinRetryBackoff,
		MaxRetryBackoff:    roleSettings.MaxRetryBackoff,
		DialerRetries:      roleSettings.DialerRetries,
		DialerRetryTimeout: roleSettings.DialerRetryTimeout,
	}
	applyPoolConfig(options, config.pool)
	return options
}

func failoverOptions(config Config, role Role) *redis.FailoverOptions {
	roleSettings := roleSettings(role)
	options := &redis.FailoverOptions{
		MasterName:         config.SentinelMasterName,
		SentinelAddrs:      append([]string(nil), config.SentinelAddrs...),
		Password:           config.Password,
		DB:                 config.DB,
		TLSConfig:          config.tlsConfig(),
		DialTimeout:        roleSettings.DialTimeout,
		ReadTimeout:        roleSettings.ReadTimeout,
		WriteTimeout:       roleSettings.WriteTimeout,
		PoolTimeout:        roleSettings.PoolTimeout,
		MaxRetries:         roleSettings.MaxRetries,
		MinRetryBackoff:    roleSettings.MinRetryBackoff,
		MaxRetryBackoff:    roleSettings.MaxRetryBackoff,
		DialerRetries:      roleSettings.DialerRetries,
		DialerRetryTimeout: roleSettings.DialerRetryTimeout,
	}
	applyFailoverPoolConfig(options, config.pool)
	return options
}

func roleSettings(role Role) roleConfig {
	switch role {
	case RoleCoordination:
		return roleConfig{
			DialTimeout:        500 * time.Millisecond,
			ReadTimeout:        500 * time.Millisecond,
			WriteTimeout:       500 * time.Millisecond,
			PoolTimeout:        750 * time.Millisecond,
			MaxRetries:         -1,
			MinRetryBackoff:    -1,
			MaxRetryBackoff:    -1,
			DialerRetries:      1,
			DialerRetryTimeout: 50 * time.Millisecond,
		}
	case RoleCache:
		return roleConfig{
			DialTimeout:        750 * time.Millisecond,
			ReadTimeout:        750 * time.Millisecond,
			WriteTimeout:       750 * time.Millisecond,
			PoolTimeout:        1 * time.Second,
			MaxRetries:         1,
			MinRetryBackoff:    25 * time.Millisecond,
			MaxRetryBackoff:    150 * time.Millisecond,
			DialerRetries:      2,
			DialerRetryTimeout: 75 * time.Millisecond,
		}
	case RoleQueue:
		return roleConfig{
			DialTimeout:        1 * time.Second,
			ReadTimeout:        2 * time.Second,
			WriteTimeout:       2 * time.Second,
			PoolTimeout:        2 * time.Second,
			MaxRetries:         2,
			MinRetryBackoff:    50 * time.Millisecond,
			MaxRetryBackoff:    250 * time.Millisecond,
			DialerRetries:      3,
			DialerRetryTimeout: 100 * time.Millisecond,
		}
	case RoleSessionContinuity:
		return roleConfig{
			DialTimeout:        750 * time.Millisecond,
			ReadTimeout:        1 * time.Second,
			WriteTimeout:       1 * time.Second,
			PoolTimeout:        1250 * time.Millisecond,
			MaxRetries:         1,
			MinRetryBackoff:    25 * time.Millisecond,
			MaxRetryBackoff:    150 * time.Millisecond,
			DialerRetries:      2,
			DialerRetryTimeout: 75 * time.Millisecond,
		}
	default:
		return roleConfig{
			DialTimeout:        1 * time.Second,
			ReadTimeout:        1 * time.Second,
			WriteTimeout:       1 * time.Second,
			PoolTimeout:        1500 * time.Millisecond,
			MaxRetries:         1,
			MinRetryBackoff:    25 * time.Millisecond,
			MaxRetryBackoff:    150 * time.Millisecond,
			DialerRetries:      2,
			DialerRetryTimeout: 75 * time.Millisecond,
		}
	}
}

func applyPoolConfig(options *redis.Options, config poolConfig) {
	if options == nil {
		return
	}
	options.PoolSize = config.PoolSize
	options.MinIdleConns = config.MinIdleConns
	options.MaxIdleConns = config.MaxIdleConns
	options.ConnMaxIdleTime = config.ConnMaxIdleTime
	options.ConnMaxLifetime = config.ConnMaxLifetime
}

func applyFailoverPoolConfig(options *redis.FailoverOptions, config poolConfig) {
	if options == nil {
		return
	}
	options.PoolSize = config.PoolSize
	options.MinIdleConns = config.MinIdleConns
	options.MaxIdleConns = config.MaxIdleConns
	options.ConnMaxIdleTime = config.ConnMaxIdleTime
	options.ConnMaxLifetime = config.ConnMaxLifetime
}

func endpointModeFromEnv() (string, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(endpointModeEnv))) {
	case "", endpointModeDirect:
		return endpointModeDirect, nil
	case endpointModeSentinel:
		return endpointModeSentinel, nil
	default:
		return "", fmt.Errorf("%s must be one of: %s, %s", endpointModeEnv, endpointModeDirect, endpointModeSentinel)
	}
}

func poolConfigFromEnv() (poolConfig, error) {
	poolSize, err := nonNegativeIntFromEnv(poolSizeEnv)
	if err != nil {
		return poolConfig{}, err
	}
	minIdleConns, err := nonNegativeIntFromEnv(minIdleConnsEnv)
	if err != nil {
		return poolConfig{}, err
	}
	maxIdleConns, err := nonNegativeIntFromEnv(maxIdleConnsEnv)
	if err != nil {
		return poolConfig{}, err
	}
	connMaxIdleTime, err := nonNegativeDurationFromEnv(connMaxIdleTimeEnv)
	if err != nil {
		return poolConfig{}, err
	}
	if strings.TrimSpace(os.Getenv(connMaxIdleTimeEnv)) != "" && connMaxIdleTime == 0 {
		return poolConfig{}, fmt.Errorf("invalid %s: must be positive; leave empty to use go-redis default idle timeout", connMaxIdleTimeEnv)
	}
	connMaxLifetime, err := nonNegativeDurationFromEnv(connMaxLifetimeEnv)
	if err != nil {
		return poolConfig{}, err
	}

	return poolConfig{
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxIdleTime: connMaxIdleTime,
		ConnMaxLifetime: connMaxLifetime,
	}, nil
}

func pingWithRetry(ctx context.Context, client *redis.Client) error {
	var lastErr error
	for range 10 {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = client.Ping(pingCtx).Err()
		cancel()
		if lastErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return lastErr
}

func (c *ClientSet) Close() error {
	if c == nil {
		return nil
	}
	clients := []*redis.Client{
		c.Default,
		c.Coordination,
		c.Cache,
		c.Queue,
		c.Session,
	}
	seen := make(map[*redis.Client]struct{}, len(clients))
	var firstErr error
	for _, client := range clients {
		if client == nil {
			continue
		}
		if _, ok := seen[client]; ok {
			continue
		}
		seen[client] = struct{}{}
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type loggingHook struct {
	role Role
}

func newLoggingHook(role Role) redis.Hook {
	return loggingHook{role: role}
}

func (h loggingHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h loggingHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		startedAt := time.Now()
		err := next(ctx, cmd)
		duration := time.Since(startedAt)
		if err == nil && duration < 750*time.Millisecond {
			return nil
		}
		role := string(h.role)
		if role == "" {
			role = string(RoleDefault)
		}
		if err != nil {
			log.Printf("redis command failed role=%s cmd=%s duration=%s err=%v", role, cmd.FullName(), duration, err)
			return err
		}
		log.Printf("redis command slow role=%s cmd=%s duration=%s", role, cmd.FullName(), duration)
		return nil
	}
}

func (h loggingHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func redisDBFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv(dbEnv))
	if raw == "" {
		return 0, nil
	}
	db, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	if db < 0 {
		return 0, fmt.Errorf("invalid REDIS_DB: must be non-negative")
	}
	return db, nil
}

func nonNegativeIntFromEnv(name string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s: must be non-negative", name)
	}
	return int(value), nil
}

func nonNegativeDurationFromEnv(name string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s: must be non-negative", name)
	}
	return value, nil
}

func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func csvEnv(name string) []string {
	raw := strings.Split(os.Getenv(name), ",")
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func redisTLSConfig(config Config) (*tls.Config, error) {
	if !config.TLS {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: config.TLSServerName,
	}
	certPool, err := redisTLSRootCAs(config.TLSCACert, config.TLSCAFile)
	if err != nil {
		return nil, err
	}
	tlsConfig.RootCAs = certPool
	return tlsConfig, nil
}

func redisTLSRootCAs(inlineCert string, certFile string) (*x509.CertPool, error) {
	if inlineCert == "" && certFile == "" {
		return nil, nil
	}
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if inlineCert != "" && !pool.AppendCertsFromPEM([]byte(inlineCert)) {
		return nil, fmt.Errorf("invalid %s: no PEM certificates found", tlsCACertEnv)
	}
	if certFile != "" {
		pemBytes, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", tlsCAFileEnv, err)
		}
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("invalid %s: no PEM certificates found", tlsCAFileEnv)
		}
	}
	return pool, nil
}

func (c Config) tlsConfig() *tls.Config {
	if !c.TLS {
		return nil
	}
	if c.tls == nil {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: c.TLSServerName,
		}
		tlsConfig.RootCAs, _ = redisTLSRootCAs(c.TLSCACert, c.TLSCAFile)
		return tlsConfig
	}
	return c.tls.Clone()
}

func (c *Config) configureTLS() error {
	if !c.TLS {
		c.tls = nil
		return nil
	}
	if c.tls != nil {
		return nil
	}
	tlsConfig, err := redisTLSConfig(*c)
	if err != nil {
		return err
	}
	c.tls = tlsConfig
	return nil
}
