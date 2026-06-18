package session

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisEndpointModeEnv       = "REDIS_ENDPOINT_MODE"
	redisAddrEnv               = "REDIS_ADDR"
	redisPasswordEnv           = "REDIS_PASSWORD"
	redisDBEnv                 = "REDIS_DB"
	redisTLSEnv                = "REDIS_TLS"
	redisSentinelAddrsEnv      = "REDIS_SENTINEL_ADDRS"
	redisSentinelMasterEnv     = "REDIS_SENTINEL_MASTER_NAME"
	redisEndpointModeDirect    = "direct"
	redisEndpointModeSentinel  = "sentinel"
	redisSentinelMasterDefault = "mpp-redis-ha"

	browserSessionKeyPrefix       = "mpp:browser:session:"
	browserSessionHeartbeatPrefix = "mpp:browser:worker-heartbeat:"
	browserSessionRedisGrace      = time.Minute
	browserSessionHeartbeatTTL    = 45 * time.Second
	HeartbeatRefreshInterval      = 15 * time.Second

	redisDialTimeout        = 750 * time.Millisecond
	redisReadTimeout        = 1 * time.Second
	redisWriteTimeout       = 1 * time.Second
	redisPoolTimeout        = 1250 * time.Millisecond
	redisMinRetryBackoff    = 25 * time.Millisecond
	redisMaxRetryBackoff    = 150 * time.Millisecond
	redisDialerRetries      = 2
	redisDialerRetryTimeout = 75 * time.Millisecond
	redisCommandRetries     = 1
)

var errRedisNotConfigured = errors.New("redis is not configured")

type RedisStateStore struct {
	client *redis.Client
}

type redisConnectionConfig struct {
	EndpointMode       string
	Addr               string
	Password           string
	DB                 int
	TLS                bool
	SentinelAddrs      []string
	SentinelMasterName string
}

type WorkerSessionState struct {
	WorkerSessionRef string    `json:"worker_session_ref"`
	Status           string    `json:"status"`
	CurrentURL       string    `json:"current_url"`
	LoginDetected    bool      `json:"login_detected"`
	MissingCookies   []string  `json:"missing_cookies"`
	Message          string    `json:"message"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type redisLiveSession struct {
	SessionID        string    `json:"session_id"`
	UserID           string    `json:"user_id"`
	TenantID         string    `json:"tenant_id,omitempty"`
	Platform         string    `json:"platform"`
	Status           string    `json:"status"`
	WorkerSessionRef string    `json:"worker_session_ref"`
	CurrentURL       string    `json:"current_url,omitempty"`
	LoginDetected    bool      `json:"login_detected,omitempty"`
	MissingCookies   []string  `json:"missing_cookies,omitempty"`
	Message          string    `json:"message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewRedisStateStoreFromEnv(ctx context.Context) (*RedisStateStore, error) {
	config, err := redisConnectionConfigFromEnv()
	if errors.Is(err, errRedisNotConfigured) {
		return &RedisStateStore{}, nil
	}
	if err != nil {
		return nil, err
	}

	client := newRedisClient(config)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisStateStore{client: client}, nil
}

func redisConnectionConfigFromEnv() (redisConnectionConfig, error) {
	endpointMode, err := redisEndpointModeFromEnv()
	if err != nil {
		return redisConnectionConfig{}, err
	}

	config := redisConnectionConfig{
		EndpointMode: endpointMode,
		Addr:         strings.TrimSpace(os.Getenv(redisAddrEnv)),
		Password:     strings.TrimSpace(os.Getenv(redisPasswordEnv)),
		TLS:          redisEnvFlagEnabled(redisTLSEnv),
	}
	switch endpointMode {
	case redisEndpointModeDirect:
		if config.Addr == "" {
			return redisConnectionConfig{}, errRedisNotConfigured
		}
	case redisEndpointModeSentinel:
		config.SentinelAddrs = redisCSVEnv(redisSentinelAddrsEnv)
		config.SentinelMasterName = strings.TrimSpace(os.Getenv(redisSentinelMasterEnv))
		if config.SentinelMasterName == "" {
			config.SentinelMasterName = redisSentinelMasterDefault
		}
		if len(config.SentinelAddrs) == 0 {
			return redisConnectionConfig{}, fmt.Errorf("%s must be set when %s=sentinel", redisSentinelAddrsEnv, redisEndpointModeEnv)
		}
	}

	db, err := redisDBFromEnv()
	if err != nil {
		return redisConnectionConfig{}, err
	}
	config.DB = db

	return config, nil
}

func newRedisClient(config redisConnectionConfig) *redis.Client {
	if config.EndpointMode == redisEndpointModeSentinel {
		return redis.NewFailoverClient(redisFailoverOptions(config))
	}
	return redis.NewClient(redisOptions(config))
}

func redisOptions(config redisConnectionConfig) *redis.Options {
	return &redis.Options{
		Addr:               config.Addr,
		Password:           config.Password,
		DB:                 config.DB,
		TLSConfig:          redisTLSConfig(config.TLS),
		DialTimeout:        redisDialTimeout,
		ReadTimeout:        redisReadTimeout,
		WriteTimeout:       redisWriteTimeout,
		PoolTimeout:        redisPoolTimeout,
		MaxRetries:         redisCommandRetries,
		MinRetryBackoff:    redisMinRetryBackoff,
		MaxRetryBackoff:    redisMaxRetryBackoff,
		DialerRetries:      redisDialerRetries,
		DialerRetryTimeout: redisDialerRetryTimeout,
	}
}

func redisFailoverOptions(config redisConnectionConfig) *redis.FailoverOptions {
	return &redis.FailoverOptions{
		MasterName:         config.SentinelMasterName,
		SentinelAddrs:      append([]string(nil), config.SentinelAddrs...),
		Password:           config.Password,
		DB:                 config.DB,
		TLSConfig:          redisTLSConfig(config.TLS),
		DialTimeout:        redisDialTimeout,
		ReadTimeout:        redisReadTimeout,
		WriteTimeout:       redisWriteTimeout,
		PoolTimeout:        redisPoolTimeout,
		MaxRetries:         redisCommandRetries,
		MinRetryBackoff:    redisMinRetryBackoff,
		MaxRetryBackoff:    redisMaxRetryBackoff,
		DialerRetries:      redisDialerRetries,
		DialerRetryTimeout: redisDialerRetryTimeout,
	}
}

func (s *RedisStateStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *RedisStateStore) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Ping(ctx).Err()
}

func (s *RedisStateStore) SaveLiveSession(ctx context.Context, session *WorkerSession, state WorkerSessionState) error {
	if s == nil || s.client == nil {
		return nil
	}
	tenantID, err := s.liveSessionTenantID(ctx, session.SessionID.String())
	if err != nil {
		return err
	}
	payload, err := json.Marshal(redisLiveSession{
		SessionID:        session.SessionID.String(),
		UserID:           session.UserID.String(),
		TenantID:         tenantID,
		Platform:         session.Platform,
		Status:           state.Status,
		WorkerSessionRef: session.ID,
		CurrentURL:       state.CurrentURL,
		LoginDetected:    state.LoginDetected,
		MissingCookies:   state.MissingCookies,
		Message:          state.Message,
		CreatedAt:        session.CreatedAt,
		ExpiresAt:        session.ExpiresAt,
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return s.client.Set(ctx, browserSessionRedisKey(session.SessionID.String()), payload, browserSessionLiveTTL(session.ExpiresAt)).Err()
}

func (s *RedisStateStore) liveSessionTenantID(ctx context.Context, sessionID string) (string, error) {
	raw, err := s.client.Get(ctx, browserSessionRedisKey(sessionID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var existing redisLiveSession
	if err := json.Unmarshal(raw, &existing); err != nil {
		return "", err
	}
	return existing.TenantID, nil
}

func (s *RedisStateStore) RefreshHeartbeat(ctx context.Context, session *WorkerSession) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Set(ctx, browserSessionHeartbeatKey(session.ID), session.SessionID.String(), browserSessionHeartbeatTTL).Err()
}

func (s *RedisStateStore) DeleteHeartbeat(ctx context.Context, workerSessionRef string) error {
	if s == nil || s.client == nil || workerSessionRef == "" {
		return nil
	}
	return s.client.Del(ctx, browserSessionHeartbeatKey(workerSessionRef)).Err()
}

func browserSessionRedisKey(sessionID string) string {
	return browserSessionKeyPrefix + sessionID
}

func browserSessionHeartbeatKey(workerSessionRef string) string {
	return browserSessionHeartbeatPrefix + workerSessionRef
}

func browserSessionLiveTTL(expiresAt time.Time) time.Duration {
	ttl := time.Until(expiresAt) + browserSessionRedisGrace
	if ttl <= 0 {
		return browserSessionRedisGrace
	}
	return ttl
}

func redisDBFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv(redisDBEnv))
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

func redisEnvFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func redisEndpointModeFromEnv() (string, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(redisEndpointModeEnv))) {
	case "", redisEndpointModeDirect:
		return redisEndpointModeDirect, nil
	case redisEndpointModeSentinel:
		return redisEndpointModeSentinel, nil
	default:
		return "", fmt.Errorf("%s must be one of: %s, %s", redisEndpointModeEnv, redisEndpointModeDirect, redisEndpointModeSentinel)
	}
}

func redisCSVEnv(name string) []string {
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

func redisTLSConfig(enabled bool) *tls.Config {
	if !enabled {
		return nil
	}
	return &tls.Config{MinVersion: tls.VersionTLS12}
}
