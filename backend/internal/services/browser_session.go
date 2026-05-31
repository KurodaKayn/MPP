package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrActiveSessionExists  = errors.New("an active session already exists for this platform")
	ErrPlatformNotSupported = errors.New("platform does not support remote browser sessions")
	ErrSessionNotFound      = errors.New("session not found")
	ErrInvalidStreamToken   = errors.New("invalid or expired stream token")
)

const (
	pendingSessionStaleAfter = 2 * time.Minute
	browserSessionTTL        = 15 * time.Minute
	browserSessionRedisGrace = 1 * time.Minute
	streamTokenMaxTTL        = 5 * time.Minute

	browserSessionActiveKeyPrefix       = "mpp:browser:active:"
	browserSessionKeyPrefix             = "mpp:browser:session:"
	browserSessionStreamTokenPrefix     = "mpp:browser:stream-token:"
	browserSessionStreamCurrentPrefix   = "mpp:browser:stream-current:"
	browserSessionWorkerHeartbeatPrefix = "mpp:browser:worker-heartbeat:"
	browserSessionCleanupKey            = "mpp:browser:cleanup"
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

type browserStreamTokenMeta struct {
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
	Platform  string    `json:"platform"`
	Purpose   string    `json:"purpose"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BrowserSessionService struct {
	db           *gorm.DB
	workerClient publisher.BrowserWorkerClient
	cookieStore  *publisher.CookieStore
	adapters     map[string]publisher.RemoteBrowserPlatformAdapter
	redisClient  *redis.Client
}

func NewBrowserSessionService(db *gorm.DB, worker publisher.BrowserWorkerClient, store *publisher.CookieStore) *BrowserSessionService {
	s := &BrowserSessionService{
		db:           db,
		workerClient: worker,
		cookieStore:  store,
		adapters:     make(map[string]publisher.RemoteBrowserPlatformAdapter),
	}
	// Register adapters
	s.RegisterAdapter(&publisher.DouyinAdapter{})
	s.RegisterAdapter(&publisher.ZhihuAdapter{})
	return s
}

func (s *BrowserSessionService) RegisterAdapter(a publisher.RemoteBrowserPlatformAdapter) {
	s.adapters[a.Platform()] = a
}

func (s *BrowserSessionService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.redisClient = client
}

func (s *BrowserSessionService) activeSessionExists(ctx context.Context, userID uuid.UUID, platform string, now time.Time) (bool, error) {
	var sessions []models.RemoteBrowserSession
	// Search for ALL sessions with active statuses (ignore expires_at for now to handle stale index rows)
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND platform = ? AND status IN ?", userID, platform, activeBrowserSessionStatuses()).
		Find(&sessions).Error
	if err != nil {
		return false, err
	}

	for i := range sessions {
		session := &sessions[i]

		// 1. If actually expired by time, mark it so and continue
		if session.ExpiresAt.Before(now) {
			if err := s.expireStaleSession(ctx, session, "session expired"); err != nil {
				return false, err
			}
			continue
		}

		// 2. If no worker ref, check for pending timeout
		if session.WorkerSessionRef == "" {
			if session.CreatedAt.Add(pendingSessionStaleAfter).After(now) {
				return true, nil
			}
			if err := s.expireStaleSession(ctx, session, "worker session reference is missing"); err != nil {
				return false, err
			}
			continue
		}

		// 3. Verify with worker
		if _, err := s.workerClient.GetSession(ctx, session.WorkerSessionRef); err != nil {
			if err := s.expireStaleSession(ctx, session, "worker session is unavailable"); err != nil {
				return false, err
			}
			continue
		}
		return true, nil
	}

	return false, nil
}

func (s *BrowserSessionService) expireStaleSession(ctx context.Context, session *models.RemoteBrowserSession, message string) error {
	return s.db.WithContext(ctx).Model(session).Updates(map[string]interface{}{
		"status":        models.BrowserSessionStatusExpired,
		"error_message": message,
	}).Error
}

func (s *BrowserSessionService) expireSupersededActiveRows(ctx context.Context, userID uuid.UUID, platform string) error {
	return s.db.WithContext(ctx).Model(&models.RemoteBrowserSession{}).
		Where("user_id = ? AND platform = ? AND status IN ?", userID, platform, activeBrowserSessionStatuses()).
		Updates(map[string]interface{}{
			"status":        models.BrowserSessionStatusExpired,
			"error_message": "superseded by redis active-session lock recovery",
		}).Error
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
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	return s.redisClient.Eval(ctx, script, []string{browserSessionActiveKey(userID, platform)}, sessionID.String()).Err()
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

func (s *BrowserSessionService) rotateRedisStreamToken(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID, platform string, tokenHash string, sessionExpiresAt time.Time) (time.Time, error) {
	expiresAt := streamTokenExpiresAt(sessionExpiresAt)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return time.Time{}, ErrInvalidStreamToken
	}
	if s.redisClient == nil {
		return expiresAt, nil
	}
	meta := browserStreamTokenMeta{
		SessionID: sessionID,
		UserID:    userID,
		Platform:  platform,
		Purpose:   "stream",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return time.Time{}, err
	}
	const script = `
local old_hash = redis.call("GET", KEYS[1])
if old_hash and old_hash ~= ARGV[1] then
	redis.call("DEL", ARGV[4] .. old_hash)
end
redis.call("SET", KEYS[2], ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[3])
return 1
`
	return expiresAt, s.redisClient.Eval(ctx, script, []string{
		browserSessionStreamCurrentKey(sessionID),
		browserSessionStreamTokenKey(sessionID, tokenHash),
	}, tokenHash, payload, ttl.Milliseconds(), browserSessionStreamTokenKeyPrefixFor(sessionID)).Err()
}

func (s *BrowserSessionService) readRedisStreamToken(ctx context.Context, sessionID uuid.UUID, tokenHash string, consume bool) (browserStreamTokenMeta, bool, error) {
	if s.redisClient == nil {
		return browserStreamTokenMeta{}, false, nil
	}
	tokenKey := browserSessionStreamTokenKey(sessionID, tokenHash)
	var raw []byte
	var err error
	if consume {
		const script = `
local payload = redis.call("GET", KEYS[1])
if not payload then
	return nil
end
redis.call("DEL", KEYS[1])
if redis.call("GET", KEYS[2]) == ARGV[1] then
	redis.call("DEL", KEYS[2])
end
return payload
`
		var result interface{}
		result, err = s.redisClient.Eval(ctx, script, []string{tokenKey, browserSessionStreamCurrentKey(sessionID)}, tokenHash).Result()
		if err == nil {
			switch value := result.(type) {
			case string:
				raw = []byte(value)
			case []byte:
				raw = value
			default:
				err = fmt.Errorf("unexpected redis stream token payload type %T", result)
			}
		}
	} else {
		raw, err = s.redisClient.Get(ctx, tokenKey).Bytes()
	}
	if errors.Is(err, redis.Nil) {
		return browserStreamTokenMeta{}, false, nil
	}
	if err != nil {
		return browserStreamTokenMeta{}, false, err
	}
	var meta browserStreamTokenMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return browserStreamTokenMeta{}, false, err
	}
	return meta, true, nil
}

func (s *BrowserSessionService) deleteRedisStreamToken(ctx context.Context, sessionID uuid.UUID) error {
	if s.redisClient == nil {
		return nil
	}
	currentHash, err := s.redisClient.Get(ctx, browserSessionStreamCurrentKey(sessionID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	keys := []string{browserSessionStreamCurrentKey(sessionID)}
	if currentHash != "" {
		keys = append(keys, browserSessionStreamTokenKey(sessionID, currentHash))
	}
	return s.redisClient.Del(ctx, keys...).Err()
}

func (s *BrowserSessionService) hasCurrentStreamToken(ctx context.Context, session models.RemoteBrowserSession) (bool, error) {
	if s.redisClient == nil {
		return session.ConnectTokenHash != "" && streamTokenValidUntil(session).After(time.Now()), nil
	}
	currentHash, err := s.redisClient.Get(ctx, browserSessionStreamCurrentKey(session.ID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if currentHash == "" {
		return false, nil
	}
	if err := s.redisClient.Get(ctx, browserSessionStreamTokenKey(session.ID, currentHash)).Err(); errors.Is(err, redis.Nil) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (s *BrowserSessionService) StartSession(ctx context.Context, userID uuid.UUID, platform string) (*dto.StartBrowserSessionResponse, error) {
	adapter, ok := s.adapters[platform]
	if !ok {
		return nil, ErrPlatformNotSupported
	}

	now := time.Now()
	sessionID := uuid.New()
	expiresAt := now.Add(browserSessionTTL)

	// 1. Use Redis as the live active-session lock when available.
	if s.redisClient != nil {
		acquired, err := s.acquireRedisActiveSession(ctx, userID, platform, sessionID, expiresAt)
		if err != nil {
			return nil, err
		}
		if !acquired {
			return nil, ErrActiveSessionExists
		}
		if err := s.expireSupersededActiveRows(ctx, userID, platform); err != nil {
			_ = s.releaseRedisActiveSession(ctx, userID, platform, sessionID)
			return nil, err
		}
	} else {
		activeSessionExists, err := s.activeSessionExists(ctx, userID, platform, now)
		if err != nil {
			return nil, err
		}
		if activeSessionExists {
			return nil, ErrActiveSessionExists
		}
	}

	// 2. Generate stream token
	token, tokenHash, err := generateStreamToken()
	if err != nil {
		_ = s.releaseRedisActiveSession(ctx, userID, platform, sessionID)
		return nil, err
	}

	// 3. Create session in DB
	session := &models.RemoteBrowserSession{
		ID:                    sessionID,
		UserID:                userID,
		Platform:              platform,
		Status:                models.BrowserSessionStatusPending,
		ConnectTokenHash:      tokenHash,
		ConnectTokenExpiresAt: streamTokenExpiresAt(expiresAt, now),
		CreatedAt:             now,
		ExpiresAt:             expiresAt,
	}

	if err := s.db.Create(session).Error; err != nil {
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, "")
		return nil, err
	}
	if err := s.saveRedisLiveSession(ctx, browserSessionLiveState{
		SessionID: sessionID,
		UserID:    userID,
		Platform:  platform,
		Status:    models.BrowserSessionStatusPending,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}); err != nil {
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, "")
		_ = s.db.Model(session).Update("status", models.BrowserSessionStatusFailed).Error
		return nil, err
	}

	// 4. Call worker
	req := publisher.StartWorkerSessionRequest{
		SessionID:       sessionID,
		UserID:          userID,
		Platform:        platform,
		LoginURL:        adapter.LoginURL(),
		AllowedDomains:  adapter.AllowedDomains(),
		RequiredCookies: adapter.RequiredCookies(),
		TTLSeconds:      900, // 15 mins
	}
	req.Viewport.Width = 1366
	req.Viewport.Height = 768

	resp, err := s.workerClient.CreateSession(ctx, req)
	if err != nil {
		// Update status to failed
		s.db.Model(session).Update("status", models.BrowserSessionStatusFailed)
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, "")
		return nil, fmt.Errorf("worker failed to create session: %w", err)
	}

	// 5. Update session with worker info
	err = s.db.Model(session).Updates(map[string]interface{}{
		"status":              models.BrowserSessionStatusReady,
		"worker_session_ref":  resp.WorkerSessionRef,
		"container_id":        resp.ContainerID,
		"cdp_endpoint_ref":    resp.CDPEndpointRef,
		"stream_endpoint_ref": resp.StreamEndpointRef,
	}).Error
	if err != nil {
		_ = s.workerClient.StopSession(ctx, resp.WorkerSessionRef)
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, resp.WorkerSessionRef)
		return nil, err
	}
	session.Status = models.BrowserSessionStatusReady
	session.WorkerSessionRef = resp.WorkerSessionRef
	session.ContainerID = resp.ContainerID
	session.CDPEndpointRef = resp.CDPEndpointRef
	session.StreamEndpointRef = resp.StreamEndpointRef

	if err := s.saveRedisLiveSession(ctx, browserSessionLiveState{
		SessionID:         sessionID,
		UserID:            userID,
		Platform:          platform,
		Status:            models.BrowserSessionStatusReady,
		WorkerSessionRef:  resp.WorkerSessionRef,
		ContainerID:       resp.ContainerID,
		CDPEndpointRef:    resp.CDPEndpointRef,
		StreamEndpointRef: resp.StreamEndpointRef,
		CreatedAt:         now,
		ExpiresAt:         expiresAt,
	}); err != nil {
		_ = s.workerClient.StopSession(ctx, resp.WorkerSessionRef)
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, resp.WorkerSessionRef)
		_ = s.db.Model(session).Update("status", models.BrowserSessionStatusFailed).Error
		return nil, err
	}

	tokenExpiresAt, err := s.rotateRedisStreamToken(ctx, sessionID, userID, platform, tokenHash, expiresAt)
	if err != nil {
		_ = s.workerClient.StopSession(ctx, resp.WorkerSessionRef)
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, resp.WorkerSessionRef)
		_ = s.db.Model(session).Update("status", models.BrowserSessionStatusFailed).Error
		return nil, err
	}
	if err := s.db.Model(session).Update("connect_token_expires_at", tokenExpiresAt).Error; err != nil {
		_ = s.workerClient.StopSession(ctx, resp.WorkerSessionRef)
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, resp.WorkerSessionRef)
		_ = s.db.Model(session).Update("status", models.BrowserSessionStatusFailed).Error
		return nil, err
	}
	session.ConnectTokenExpiresAt = tokenExpiresAt

	return &dto.StartBrowserSessionResponse{
		SessionID:            sessionID,
		Status:               models.BrowserSessionStatusReady,
		StreamURL:            browserSessionStreamURL(sessionID, token),
		StreamTokenExpiresAt: tokenExpiresAt,
		ExpiresAt:            expiresAt,
	}, nil
}

func (s *BrowserSessionService) GetSession(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*dto.BrowserSessionResponse, error) {
	var session models.RemoteBrowserSession
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	if state, ok, err := s.getRedisLiveSession(ctx, id); err != nil {
		return nil, err
	} else if ok {
		session.Status = state.Status
		session.WorkerSessionRef = state.WorkerSessionRef
		session.ContainerID = state.ContainerID
		session.CDPEndpointRef = state.CDPEndpointRef
		session.StreamEndpointRef = state.StreamEndpointRef
		session.ErrorMessage = state.Message
		session.ExpiresAt = state.ExpiresAt

		if state.WorkerSessionRef != "" && !isTerminalBrowserSessionStatus(state.Status) {
			heartbeatAlive, err := s.redisWorkerHeartbeatAlive(ctx, state.WorkerSessionRef)
			if err != nil {
				return nil, err
			}
			workerState, workerErr := s.workerClient.GetSession(ctx, state.WorkerSessionRef)
			if workerErr != nil {
				nextStatus := models.BrowserSessionStatusFailed
				message := "worker session is unavailable"
				if !heartbeatAlive {
					message = "worker heartbeat missing"
				}
				if time.Now().After(state.ExpiresAt) {
					nextStatus = models.BrowserSessionStatusExpired
					message = "session expired"
				}
				session.Status = nextStatus
				session.ErrorMessage = message
				_ = s.db.Model(&session).Updates(map[string]interface{}{
					"status":             nextStatus,
					"error_message":      message,
					"connect_token_hash": "",
				}).Error
				_ = s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, state.WorkerSessionRef)
			} else {
				nextStatus := state.Status
				if workerState.LoginDetected {
					nextStatus = models.BrowserSessionStatusLoginDetected
				}
				state.Status = nextStatus
				state.CurrentURL = workerState.CurrentURL
				state.LoginDetected = workerState.LoginDetected
				state.MissingCookies = workerState.MissingCookies
				state.Message = workerState.Message
				_ = s.saveRedisLiveSession(ctx, state)
				if nextStatus != session.Status {
					_ = s.db.Model(&session).Update("status", nextStatus).Error
				}
				session.Status = nextStatus
				session.ErrorMessage = workerState.Message
			}
		}
	}

	// If expired, check worker if we should update status
	if time.Now().After(session.ExpiresAt) && session.Status != models.BrowserSessionStatusExpired {
		s.CancelSession(ctx, userID, id)
		session.Status = models.BrowserSessionStatusExpired
	}

	resp := &dto.BrowserSessionResponse{
		SessionID: id,
		Platform:  session.Platform,
		Status:    session.Status,
		ExpiresAt: session.ExpiresAt,
		Message:   session.ErrorMessage,
	}

	hasCurrentToken, err := s.hasCurrentStreamToken(ctx, session)
	if err != nil {
		return nil, err
	}
	if isStreamableBrowserSessionStatus(session.Status) && session.StreamEndpointRef != "" && !hasCurrentToken {
		token, tokenHash, err := generateStreamToken()
		if err != nil {
			return nil, err
		}
		tokenExpiresAt, err := s.rotateRedisStreamToken(ctx, id, userID, session.Platform, tokenHash, session.ExpiresAt)
		if err != nil {
			return nil, err
		}
		if s.redisClient == nil {
			if err := s.db.Model(&session).Updates(map[string]interface{}{
				"connect_token_hash":       tokenHash,
				"connect_token_expires_at": tokenExpiresAt,
			}).Error; err != nil {
				return nil, err
			}
			session.ConnectTokenHash = tokenHash
			session.ConnectTokenExpiresAt = tokenExpiresAt
		}
		resp.StreamURL = browserSessionStreamURL(id, token)
		resp.StreamTokenExpiresAt = tokenExpiresAt
	}

	return resp, nil
}

func (s *BrowserSessionService) GetStreamEndpoint(ctx context.Context, userID uuid.UUID, id uuid.UUID, token string, consume bool) (string, error) {
	if token == "" {
		return "", ErrInvalidStreamToken
	}

	var session models.RemoteBrowserSession
	query := s.db.WithContext(ctx).Where("id = ?", id)
	// Only filter by userID if it's provided (not uuid.Nil)
	if userID != uuid.Nil {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrSessionNotFound
		}
		return "", err
	}

	now := time.Now()
	if now.After(session.ExpiresAt) {
		return "", ErrInvalidStreamToken
	}
	if !isStreamableBrowserSessionStatus(session.Status) {
		return "", ErrInvalidStreamToken
	}

	tokenHash := hashStreamToken(token)
	if s.redisClient != nil {
		meta, ok, err := s.readRedisStreamToken(ctx, id, tokenHash, consume)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrInvalidStreamToken
		}
		if meta.SessionID != id || meta.Platform != session.Platform || meta.Purpose != "stream" {
			return "", ErrInvalidStreamToken
		}
		if userID != uuid.Nil && meta.UserID != userID {
			return "", ErrInvalidStreamToken
		}
		if time.Now().After(meta.ExpiresAt) {
			return "", ErrInvalidStreamToken
		}
	} else {
		if !streamTokenValidUntil(session).After(now) {
			return "", ErrInvalidStreamToken
		}
		if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(session.ConnectTokenHash)) != 1 {
			return "", ErrInvalidStreamToken
		}
		if consume {
			if err := s.db.Model(&session).Update("connect_token_hash", "").Error; err != nil {
				return "", err
			}
		}
	}

	if session.StreamEndpointRef == "" {
		return "", ErrSessionNotFound
	}

	return session.StreamEndpointRef, nil
}

func (s *BrowserSessionService) CompleteSession(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*dto.CompleteBrowserSessionResponse, error) {
	var session models.RemoteBrowserSession
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&session).Error; err != nil {
		return nil, ErrSessionNotFound
	}

	if session.Status == models.BrowserSessionStatusConnected {
		return nil, errors.New("session already completed")
	}
	if !isStreamableBrowserSessionStatus(session.Status) {
		return nil, fmt.Errorf("session is not ready for capture")
	}

	// 1. Transition to capturing
	s.db.Model(&session).Update("status", models.BrowserSessionStatusCapturing)
	_ = s.saveRedisLiveSession(ctx, browserSessionLiveState{
		SessionID:         session.ID,
		UserID:            session.UserID,
		Platform:          session.Platform,
		Status:            models.BrowserSessionStatusCapturing,
		WorkerSessionRef:  session.WorkerSessionRef,
		ContainerID:       session.ContainerID,
		CDPEndpointRef:    session.CDPEndpointRef,
		StreamEndpointRef: session.StreamEndpointRef,
		CreatedAt:         session.CreatedAt,
		ExpiresAt:         session.ExpiresAt,
	})

	// 2. Ask worker to capture
	captureResp, err := s.workerClient.CaptureSession(ctx, session.WorkerSessionRef)
	if err != nil {
		s.db.Model(&session).Updates(map[string]interface{}{
			"status":        models.BrowserSessionStatusReady,
			"error_message": err.Error(),
		})
		_ = s.saveRedisLiveSession(ctx, browserSessionLiveState{
			SessionID:         session.ID,
			UserID:            session.UserID,
			Platform:          session.Platform,
			Status:            models.BrowserSessionStatusReady,
			WorkerSessionRef:  session.WorkerSessionRef,
			ContainerID:       session.ContainerID,
			CDPEndpointRef:    session.CDPEndpointRef,
			StreamEndpointRef: session.StreamEndpointRef,
			Message:           err.Error(),
			CreatedAt:         session.CreatedAt,
			ExpiresAt:         session.ExpiresAt,
		})
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	if captureResp.Status != "login_detected" {
		message := "login not detected yet"
		if len(captureResp.MissingCookies) > 0 {
			message = "missing required cookies: " + strings.Join(captureResp.MissingCookies, ", ")
		}
		s.db.Model(&session).Update("status", models.BrowserSessionStatusReady)
		_ = s.saveRedisLiveSession(ctx, browserSessionLiveState{
			SessionID:         session.ID,
			UserID:            session.UserID,
			Platform:          session.Platform,
			Status:            models.BrowserSessionStatusReady,
			WorkerSessionRef:  session.WorkerSessionRef,
			ContainerID:       session.ContainerID,
			CDPEndpointRef:    session.CDPEndpointRef,
			StreamEndpointRef: session.StreamEndpointRef,
			Message:           message,
			CreatedAt:         session.CreatedAt,
			ExpiresAt:         session.ExpiresAt,
		})
		return nil, errors.New(message)
	}

	// 3. Save cookies via CookieStore
	profile := publisher.RemoteAccountProfile{
		Username:  captureResp.Account.Username,
		AvatarURL: captureResp.Account.AvatarURL,
	}
	err = s.cookieStore.Save(ctx, userID, session.Platform, captureResp.Cookies, profile)
	if err != nil {
		s.db.Model(&session).Update("status", models.BrowserSessionStatusReady)
		_ = s.saveRedisLiveSession(ctx, browserSessionLiveState{
			SessionID:         session.ID,
			UserID:            session.UserID,
			Platform:          session.Platform,
			Status:            models.BrowserSessionStatusReady,
			WorkerSessionRef:  session.WorkerSessionRef,
			ContainerID:       session.ContainerID,
			CDPEndpointRef:    session.CDPEndpointRef,
			StreamEndpointRef: session.StreamEndpointRef,
			Message:           err.Error(),
			CreatedAt:         session.CreatedAt,
			ExpiresAt:         session.ExpiresAt,
		})
		return nil, fmt.Errorf("failed to save cookies: %w", err)
	}

	// 4. Finalize session
	now := time.Now()
	s.db.Model(&session).Updates(map[string]interface{}{
		"status":       models.BrowserSessionStatusConnected,
		"completed_at": &now,
	})

	// 5. Stop worker
	s.workerClient.StopSession(ctx, session.WorkerSessionRef)
	_ = s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)

	return &dto.CompleteBrowserSessionResponse{
		SessionID: id,
		Platform:  session.Platform,
		Status:    models.BrowserSessionStatusConnected,
		Account: struct {
			Username  string `json:"username"`
			AvatarURL string `json:"avatar_url"`
		}{
			Username:  profile.Username,
			AvatarURL: profile.AvatarURL,
		},
		Message: "Account connected successfully",
	}, nil
}

func (s *BrowserSessionService) CancelSession(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	var session models.RemoteBrowserSession
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&session).Error; err != nil {
		return ErrSessionNotFound
	}

	if session.WorkerSessionRef != "" {
		s.workerClient.StopSession(ctx, session.WorkerSessionRef)
	}
	_ = s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)

	return s.db.Model(&session).Updates(map[string]interface{}{
		"status":             models.BrowserSessionStatusExpired,
		"connect_token_hash": "",
	}).Error
}

func (s *BrowserSessionService) StartCleanupWorker(ctx context.Context) {
	if s.redisClient == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := s.CleanupExpiredSessions(ctx, time.Now()); err != nil {
				log.Printf("browser session cleanup failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *BrowserSessionService) CleanupExpiredSessions(ctx context.Context, now time.Time) error {
	if s.redisClient == nil {
		return nil
	}
	sessionIDs, err := s.redisClient.ZRangeByScore(ctx, browserSessionCleanupKey, &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%d", now.UnixMilli()),
	}).Result()
	if err != nil {
		return err
	}
	for _, rawID := range sessionIDs {
		sessionID, err := uuid.Parse(rawID)
		if err != nil {
			_ = s.redisClient.ZRem(ctx, browserSessionCleanupKey, rawID).Err()
			continue
		}
		if err := s.cleanupExpiredSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func (s *BrowserSessionService) cleanupExpiredSession(ctx context.Context, sessionID uuid.UUID) error {
	var session models.RemoteBrowserSession
	if err := s.db.WithContext(ctx).First(&session, sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.removeRedisCleanupMember(ctx, sessionID)
		}
		return err
	}
	if isTerminalBrowserSessionStatus(session.Status) {
		return s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)
	}
	if session.WorkerSessionRef != "" {
		_ = s.workerClient.StopSession(ctx, session.WorkerSessionRef)
	}
	if err := s.db.WithContext(ctx).Model(&session).Updates(map[string]interface{}{
		"status":             models.BrowserSessionStatusExpired,
		"error_message":      "session expired",
		"connect_token_hash": "",
	}).Error; err != nil {
		return err
	}
	return s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)
}

func browserSessionStreamURL(sessionID uuid.UUID, token string) string {
	streamBasePath := fmt.Sprintf(
		"api/browser-stream/%s/%s",
		sessionID,
		url.PathEscape(token),
	)
	query := url.Values{
		"autoconnect": {"true"},
		"path":        {streamBasePath + "/websockify"},
		"resize":      {"scale"},
	}
	return fmt.Sprintf("/%s/vnc.html?%s", streamBasePath, query.Encode())
}

func streamTokenExpiresAt(sessionExpiresAt time.Time, issuedAt ...time.Time) time.Time {
	now := time.Now()
	if len(issuedAt) > 0 && !issuedAt[0].IsZero() {
		now = issuedAt[0]
	}
	ttl := sessionExpiresAt.Sub(now)
	if ttl > streamTokenMaxTTL {
		ttl = streamTokenMaxTTL
	}
	if ttl <= 0 {
		return now
	}
	return now.Add(ttl)
}

func streamTokenValidUntil(session models.RemoteBrowserSession) time.Time {
	if !session.ConnectTokenExpiresAt.IsZero() {
		return session.ConnectTokenExpiresAt
	}
	return streamTokenExpiresAt(session.ExpiresAt, session.CreatedAt)
}

func isStreamableBrowserSessionStatus(status string) bool {
	switch status {
	case models.BrowserSessionStatusReady,
		models.BrowserSessionStatusLoginDetected,
		models.BrowserSessionStatusCapturing:
		return true
	default:
		return false
	}
}

func isTerminalBrowserSessionStatus(status string) bool {
	switch status {
	case models.BrowserSessionStatusConnected,
		models.BrowserSessionStatusExpired,
		models.BrowserSessionStatusFailed:
		return true
	default:
		return false
	}
}

func activeBrowserSessionStatuses() []string {
	return []string{
		models.BrowserSessionStatusPending,
		models.BrowserSessionStatusReady,
		models.BrowserSessionStatusLoginDetected,
		models.BrowserSessionStatusCapturing,
	}
}

func generateStreamToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, hashStreamToken(token), nil
}

func hashStreamToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}
