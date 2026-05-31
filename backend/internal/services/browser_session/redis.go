package browsersession

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/redis/go-redis/v9"
)

type browserSessionLiveState struct {
	SessionID         uuid.UUID `json:"session_id"`
	UserID            uuid.UUID `json:"user_id"`
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
	if s.redisClient == nil {
		return nil
	}
	if err := s.releaseRedisActiveSession(ctx, userID, platform, sessionID); err != nil {
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

func (s *BrowserSessionService) saveRedisLiveSession(ctx context.Context, state browserSessionLiveState) error {
	if s.redisClient == nil {
		return nil
	}
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
	return s.db.WithContext(ctx).Model(&models.RemoteBrowserSession{}).
		Where("id = ? AND status IN ?", sessionID, activeBrowserSessionStatuses()).
		Updates(map[string]interface{}{
			"status":             models.BrowserSessionStatusExpired,
			"error_message":      message,
			"connect_token_hash": "",
		}).Error
}
