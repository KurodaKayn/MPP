package browsersession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

type browserSessionLiveState struct {
	SessionID         uuid.UUID `json:"session_id"`
	UserID            uuid.UUID `json:"user_id"`
	TenantID          string    `json:"tenant_id"`
	Platform          string    `json:"platform"`
	Status            string    `json:"status"`
	WorkerSessionRef  string    `json:"worker_session_ref"`
	ContainerID       string    `json:"container_id"`
	CDPEndpointRef    string    `json:"cdp_endpoint_ref"`
	StreamEndpointRef string    `json:"stream_endpoint_ref"`
	CurrentURL        string    `json:"current_url"`
	LoginDetected     bool      `json:"login_detected"`
	MissingCookies    []string  `json:"missing_cookies"`
	Message           string    `json:"message"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (s *BrowserSessionService) cleanupRedisSession(ctx context.Context, userID uuid.UUID, platform string, sessionID uuid.UUID, workerSessionRef string) error {
	return s.cleanupRedisSessionForTenant(ctx, userID, "", platform, sessionID, workerSessionRef)
}

func (s *BrowserSessionService) cleanupRedisSessionForTenant(ctx context.Context, userID uuid.UUID, tenantID string, platform string, sessionID uuid.UUID, workerSessionRef string) error {
	if s.redisClient == nil {
		return nil
	}
	tenantID, err := s.redisSessionTenantID(ctx, tenantID, sessionID)
	if err != nil {
		return err
	}
	if err := s.releaseRedisActiveSession(ctx, userID, platform, sessionID); err != nil {
		return err
	}
	if err := s.releaseRedisConcurrencyQuota(ctx, userID, tenantID, sessionID); err != nil {
		return err
	}
	if err := s.deleteRedisStreamToken(ctx, sessionID); err != nil {
		return err
	}
	if err := s.deleteRedisLiveSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.deleteRedisWorkerHeartbeat(ctx, workerSessionRef); err != nil {
		return err
	}
	return s.removeRedisCleanupMember(ctx, sessionID)
}

func browserSessionActiveKey(userID uuid.UUID, platform string) string {
	return browserSessionActiveKeyPrefix + userID.String() + ":" + platform
}

func browserSessionQuotaUserKey(userID uuid.UUID) string {
	return browserSessionQuotaUserPrefix + userID.String()
}

func browserSessionQuotaTenantKey(tenantID string) string {
	return browserSessionQuotaTenantPrefix + normalizeBrowserSessionTenantID(tenantID)
}

func browserSessionKey(sessionID uuid.UUID) string {
	return browserSessionKeyPrefix + sessionID.String()
}

func browserSessionStreamTokenKey(sessionID uuid.UUID, tokenHash string) string {
	return browserSessionStreamTokenPrefix + sessionID.String() + ":" + tokenHash
}

func browserSessionStreamTokenKeyPrefixFor(sessionID uuid.UUID) string {
	return browserSessionStreamTokenPrefix + sessionID.String() + ":"
}

func browserSessionStreamCurrentKey(sessionID uuid.UUID) string {
	return browserSessionStreamCurrentPrefix + sessionID.String()
}

func browserSessionWorkerHeartbeatKey(workerSessionRef string) string {
	return browserSessionWorkerHeartbeatPrefix + workerSessionRef
}

func browserSessionLiveTTL(expiresAt time.Time) time.Duration {
	ttl := time.Until(expiresAt) + browserSessionRedisGrace
	if ttl <= 0 {
		return browserSessionRedisGrace
	}
	return ttl
}

func (s *BrowserSessionService) acquireRedisActiveSession(ctx context.Context, userID uuid.UUID, platform string, sessionID uuid.UUID, expiresAt time.Time) (bool, error) {
	if s.redisClient == nil {
		return true, nil
	}
	return s.redisClient.SetNX(ctx, browserSessionActiveKey(userID, platform), sessionID.String(), browserSessionLiveTTL(expiresAt)).Result()
}

func (s *BrowserSessionService) releaseRedisActiveSession(ctx context.Context, userID uuid.UUID, platform string, sessionID uuid.UUID) error {
	if s.redisClient == nil {
		return nil
	}
	return s.releaseRedisActiveSessionValue(ctx, userID, platform, sessionID.String())
}

func (s *BrowserSessionService) releaseRedisActiveSessionValue(ctx context.Context, userID uuid.UUID, platform string, activeValue string) error {
	if s.redisClient == nil {
		return nil
	}
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	return s.redisClient.Eval(ctx, script, []string{browserSessionActiveKey(userID, platform)}, activeValue).Err()
}

func (s *BrowserSessionService) acquireRedisConcurrencyQuota(ctx context.Context, userID uuid.UUID, tenantID string, sessionID uuid.UUID, expiresAt time.Time) error {
	if s.redisClient == nil {
		return nil
	}
	config := s.quotaConfig
	if config.UserConcurrencyLimit == 0 && config.TenantConcurrencyLimit == 0 {
		return nil
	}
	ttl := browserSessionLiveTTL(expiresAt)
	if ttl <= 0 {
		ttl = browserSessionRedisGrace
	}
	const script = `
local now_ms = tonumber(ARGV[1])
local expires_at_ms = tonumber(ARGV[2])
local ttl_ms = tonumber(ARGV[3])
local session_id = ARGV[4]
local user_limit = tonumber(ARGV[5])
local tenant_limit = tonumber(ARGV[6])

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now_ms)
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", now_ms)

if user_limit > 0 and redis.call("ZCARD", KEYS[1]) >= user_limit then
	return "user"
end
if tenant_limit > 0 and redis.call("ZCARD", KEYS[2]) >= tenant_limit then
	return "tenant"
end

redis.call("ZADD", KEYS[1], expires_at_ms, session_id)
redis.call("PEXPIRE", KEYS[1], ttl_ms)
redis.call("ZADD", KEYS[2], expires_at_ms, session_id)
redis.call("PEXPIRE", KEYS[2], ttl_ms)
return "ok"
`
	result, err := s.redisClient.Eval(ctx, script, []string{
		browserSessionQuotaUserKey(userID),
		browserSessionQuotaTenantKey(tenantID),
	},
		time.Now().UnixMilli(),
		expiresAt.UnixMilli(),
		ttl.Milliseconds(),
		sessionID.String(),
		config.UserConcurrencyLimit,
		config.TenantConcurrencyLimit,
	).Result()
	if err != nil {
		return err
	}
	switch fmt.Sprint(result) {
	case "ok":
		return nil
	case "user":
		return ErrUserQuotaExceeded
	case "tenant":
		return ErrTenantQuotaExceeded
	default:
		return fmt.Errorf("unexpected browser session quota result: %v", result)
	}
}

func (s *BrowserSessionService) releaseRedisConcurrencyQuota(ctx context.Context, userID uuid.UUID, tenantID string, sessionID uuid.UUID) error {
	if s.redisClient == nil {
		return nil
	}
	_, err := s.redisClient.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRem(ctx, browserSessionQuotaUserKey(userID), sessionID.String())
		pipe.ZRem(ctx, browserSessionQuotaTenantKey(tenantID), sessionID.String())
		return nil
	})
	return err
}

func (s *BrowserSessionService) saveRedisLiveSession(ctx context.Context, state browserSessionLiveState) error {
	if s.redisClient == nil {
		return nil
	}
	if state.TenantID == "" {
		existingState, ok, err := s.getRedisLiveSession(ctx, state.SessionID)
		if err != nil {
			return err
		}
		if ok {
			state.TenantID = existingState.TenantID
		}
	}
	state.TenantID = normalizeBrowserSessionTenantID(state.TenantID)
	state.UpdatedAt = time.Now().UTC()
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := s.redisClient.Set(ctx, browserSessionKey(state.SessionID), payload, browserSessionLiveTTL(state.ExpiresAt)).Err(); err != nil {
		return err
	}
	return s.redisClient.ZAdd(ctx, browserSessionCleanupKey, redis.Z{
		Score:  float64(state.ExpiresAt.UnixMilli()),
		Member: state.SessionID.String(),
	}).Err()
}

func (s *BrowserSessionService) getRedisLiveSession(ctx context.Context, sessionID uuid.UUID) (browserSessionLiveState, bool, error) {
	if s.redisClient == nil {
		return browserSessionLiveState{}, false, nil
	}
	raw, err := s.redisClient.Get(ctx, browserSessionKey(sessionID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return browserSessionLiveState{}, false, nil
	}
	if err != nil {
		return browserSessionLiveState{}, false, err
	}
	var state browserSessionLiveState
	if err := json.Unmarshal(raw, &state); err != nil {
		return browserSessionLiveState{}, false, err
	}
	return state, true, nil
}

func (s *BrowserSessionService) redisSessionTenantID(ctx context.Context, tenantID string, sessionID uuid.UUID) (string, error) {
	tenantID = normalizeBrowserSessionTenantID(tenantID)
	if tenantID != browserSessionDefaultTenantID {
		return tenantID, nil
	}
	state, ok, err := s.getRedisLiveSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if ok && state.TenantID != "" {
		return normalizeBrowserSessionTenantID(state.TenantID), nil
	}
	return tenantID, nil
}

func (s *BrowserSessionService) deleteRedisLiveSession(ctx context.Context, sessionID uuid.UUID) error {
	if s.redisClient == nil {
		return nil
	}
	return s.redisClient.Del(ctx, browserSessionKey(sessionID)).Err()
}

func (s *BrowserSessionService) redisWorkerHeartbeatAlive(ctx context.Context, workerSessionRef string) (bool, error) {
	if s.redisClient == nil || workerSessionRef == "" {
		return true, nil
	}
	exists, err := s.redisClient.Exists(ctx, browserSessionWorkerHeartbeatKey(workerSessionRef)).Result()
	return exists > 0, err
}

func (s *BrowserSessionService) deleteRedisWorkerHeartbeat(ctx context.Context, workerSessionRef string) error {
	if s.redisClient == nil || workerSessionRef == "" {
		return nil
	}
	return s.redisClient.Del(ctx, browserSessionWorkerHeartbeatKey(workerSessionRef)).Err()
}

func (s *BrowserSessionService) removeRedisCleanupMember(ctx context.Context, sessionID uuid.UUID) error {
	if s.redisClient == nil {
		return nil
	}
	return s.redisClient.ZRem(ctx, browserSessionCleanupKey, sessionID.String()).Err()
}

func (s *BrowserSessionService) recoverRedisActiveSessionLock(ctx context.Context, userID uuid.UUID, platform string, now time.Time) (bool, error) {
	if s.redisClient == nil {
		return false, nil
	}

	activeValue, err := s.redisClient.Get(ctx, browserSessionActiveKey(userID, platform)).Result()
	if errors.Is(err, redis.Nil) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	activeSessionID, err := uuid.Parse(activeValue)
	if err != nil {
		return true, s.releaseRedisActiveSessionValue(ctx, userID, platform, activeValue)
	}

	state, ok, err := s.getRedisLiveSession(ctx, activeSessionID)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, s.cleanupRecoveredRedisActiveSession(ctx, userID, platform, activeSessionID, "", "redis live session missing")
	}

	if state.UserID != userID || state.Platform != platform {
		return true, s.releaseRedisActiveSessionValue(ctx, userID, platform, activeValue)
	}
	if state.ExpiresAt.Before(now) || isTerminalBrowserSessionStatus(state.Status) {
		return true, s.cleanupRecoveredRedisActiveSession(ctx, userID, platform, activeSessionID, state.WorkerSessionRef, "session expired")
	}
	if state.WorkerSessionRef == "" {
		if state.CreatedAt.Add(pendingSessionStaleAfter).After(now) {
			return false, nil
		}
		return true, s.cleanupRecoveredRedisActiveSession(ctx, userID, platform, activeSessionID, "", "worker session reference is missing")
	}

	heartbeatAlive, err := s.redisWorkerHeartbeatAlive(ctx, state.WorkerSessionRef)
	if err != nil {
		return false, err
	}
	if _, err := s.workerClient.GetSession(ctx, state.WorkerSessionRef); err == nil {
		return false, nil
	}

	message := "worker session is unavailable"
	if !heartbeatAlive {
		message = "worker heartbeat missing"
	}
	return true, s.cleanupRecoveredRedisActiveSession(ctx, userID, platform, activeSessionID, state.WorkerSessionRef, message)
}

func (s *BrowserSessionService) cleanupRecoveredRedisActiveSession(ctx context.Context, userID uuid.UUID, platform string, sessionID uuid.UUID, workerSessionRef string, message string) error {
	if err := s.expireRedisActiveSessionRow(ctx, sessionID, message); err != nil {
		return err
	}
	return s.cleanupRedisSession(ctx, userID, platform, sessionID, workerSessionRef)
}

func (s *BrowserSessionService) expireRedisActiveSessionRow(ctx context.Context, sessionID uuid.UUID, message string) error {
	return s.writerDB(ctx).Model(&models.RemoteBrowserSession{}).
		Where("id = ? AND status IN ?", sessionID, activeBrowserSessionStatuses()).
		Updates(map[string]any{
			"status":             models.BrowserSessionStatusExpired,
			"error_message":      message,
			"connect_token_hash": "",
		}).Error
}
