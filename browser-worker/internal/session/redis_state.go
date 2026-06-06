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

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

const (
	redisAddrEnv     = "REDIS_ADDR"
	redisPasswordEnv = "REDIS_PASSWORD"
	redisDBEnv       = "REDIS_DB"
	redisTLSEnv      = "REDIS_TLS"

	browserSessionKeyPrefix       = "mpp:browser:session:"
	browserSessionHeartbeatPrefix = "mpp:browser:worker-heartbeat:"
	browserSessionRedisGrace      = time.Minute
	browserSessionHeartbeatTTL    = 45 * time.Second
	HeartbeatRefreshInterval      = 15 * time.Second
)

type RedisStateStore struct {
	client *redis.Client
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
	SessionID         string                          `json:"session_id"`
	UserID            string                          `json:"user_id"`
	TenantID          string                          `json:"tenant_id"`
	Platform          string                          `json:"platform"`
	Status            string                          `json:"status"`
	WorkerSessionRef  string                          `json:"worker_session_ref"`
	RuntimeReference  browserruntime.SessionReference `json:"runtime_reference"`
	ContainerID       string                          `json:"container_id"`
	CDPEndpointRef    string                          `json:"cdp_endpoint_ref"`
	StreamEndpointRef string                          `json:"stream_endpoint_ref"`
	CurrentURL        string                          `json:"current_url"`
	LoginDetected     bool                            `json:"login_detected"`
	MissingCookies    []string                        `json:"missing_cookies"`
	Message           string                          `json:"message"`
	CreatedAt         time.Time                       `json:"created_at"`
	ExpiresAt         time.Time                       `json:"expires_at"`
	UpdatedAt         time.Time                       `json:"updated_at"`
}

func NewRedisStateStoreFromEnv(ctx context.Context) (*RedisStateStore, error) {
	addr := strings.TrimSpace(os.Getenv(redisAddrEnv))
	if addr == "" {
		return &RedisStateStore{}, nil
	}

	db, err := redisDBFromEnv()
	if err != nil {
		return nil, err
	}

	options := &redis.Options{
		Addr:     addr,
		Password: strings.TrimSpace(os.Getenv(redisPasswordEnv)),
		DB:       db,
	}
	if redisEnvFlagEnabled(redisTLSEnv) {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisStateStore{client: client}, nil
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
		SessionID:         session.SessionID.String(),
		UserID:            session.UserID.String(),
		TenantID:          tenantID,
		Platform:          session.Platform,
		Status:            state.Status,
		WorkerSessionRef:  session.ID,
		RuntimeReference:  session.RuntimeReference,
		ContainerID:       session.ContainerID,
		CDPEndpointRef:    session.CDPEndpointRef,
		StreamEndpointRef: session.StreamEndpointRef,
		CurrentURL:        state.CurrentURL,
		LoginDetected:     state.LoginDetected,
		MissingCookies:    state.MissingCookies,
		Message:           state.Message,
		CreatedAt:         session.CreatedAt,
		ExpiresAt:         session.ExpiresAt,
		UpdatedAt:         time.Now().UTC(),
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
