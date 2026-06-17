package redisclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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
	endpointModeDirect   = "direct"
	endpointModeSentinel = "sentinel"
)

type Config struct {
	EndpointMode       string
	Addr               string
	Password           string
	DB                 int
	TLS                bool
	SentinelAddrs      []string
	SentinelMasterName string
	pool               poolConfig
}

type poolConfig struct {
	PoolSize        int
	MinIdleConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

func NewFromEnv(ctx context.Context) (*redis.Client, error) {
	config, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}

	return New(ctx, config)
}

func ConfigFromEnv() (Config, error) {
	endpointMode, err := endpointModeFromEnv()
	if err != nil {
		return Config{}, err
	}

	config := Config{
		EndpointMode: endpointMode,
		Addr:         strings.TrimSpace(os.Getenv(addrEnv)),
		Password:     strings.TrimSpace(os.Getenv(passwordEnv)),
		TLS:          envFlagEnabled(tlsEnv),
	}
	switch endpointMode {
	case endpointModeDirect:
		if config.Addr == "" {
			return Config{}, ErrNotConfigured
		}
	case endpointModeSentinel:
		config.SentinelAddrs = csvEnv(sentinelAddrsEnv)
		config.SentinelMasterName = strings.TrimSpace(os.Getenv(sentinelMasterEnv))
		if len(config.SentinelAddrs) == 0 {
			return Config{}, fmt.Errorf("%s must be set when %s=sentinel", sentinelAddrsEnv, endpointModeEnv)
		}
		if config.SentinelMasterName == "" {
			return Config{}, fmt.Errorf("%s must be set when %s=sentinel", sentinelMasterEnv, endpointModeEnv)
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

	return config, nil
}

func New(ctx context.Context, config Config) (*redis.Client, error) {
	client := newClient(config)
	if err := pingWithRetry(ctx, client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}

func newClient(config Config) *redis.Client {
	if config.EndpointMode == endpointModeSentinel {
		return redis.NewFailoverClient(failoverOptions(config))
	}
	return redis.NewClient(options(config))
}

func options(config Config) *redis.Options {
	options := &redis.Options{
		Addr:      config.Addr,
		Password:  config.Password,
		DB:        config.DB,
		TLSConfig: tlsConfig(config.TLS),
	}
	applyPoolConfig(options, config.pool)
	return options
}

func failoverOptions(config Config) *redis.FailoverOptions {
	options := &redis.FailoverOptions{
		MasterName:    config.SentinelMasterName,
		SentinelAddrs: append([]string(nil), config.SentinelAddrs...),
		Password:      config.Password,
		DB:            config.DB,
		TLSConfig:     tlsConfig(config.TLS),
	}
	applyFailoverPoolConfig(options, config.pool)
	return options
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

func tlsConfig(enabled bool) *tls.Config {
	if !enabled {
		return nil
	}
	return &tls.Config{MinVersion: tls.VersionTLS12}
}
