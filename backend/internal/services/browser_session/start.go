package browsersession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
)

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
			recovered, err := s.recoverRedisActiveSessionLock(ctx, userID, platform, now)
			if err != nil {
				return nil, err
			}
			if !recovered {
				return nil, ErrActiveSessionExists
			}
			acquired, err = s.acquireRedisActiveSession(ctx, userID, platform, sessionID, expiresAt)
			if err != nil {
				return nil, err
			}
			if !acquired {
				return nil, ErrActiveSessionExists
			}
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
	token, tokenHash, err := GenerateStreamToken()
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
		ConnectTokenExpiresAt: StreamTokenExpiresAt(expiresAt, now),
		CreatedAt:             now,
		ExpiresAt:             expiresAt,
	}

	if err := s.db.Create(session).Error; err != nil {
		_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, "")
		if isActiveSessionUniquenessError(err) {
			return nil, ErrActiveSessionExists
		}
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
	if s.cookieStore != nil {
		cookies, err := s.cookieStore.Load(ctx, userID, platform)
		if err != nil && !errors.Is(err, publisher.ErrCookieNotFound) && !errors.Is(err, publisher.ErrCookieValidationFailed) {
			_ = s.db.Model(session).Update("status", models.BrowserSessionStatusFailed)
			_ = s.cleanupRedisSession(ctx, userID, platform, sessionID, "")
			return nil, err
		}
		req.InitialCookies = cookies
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
		StreamURL:            BrowserSessionStreamURL(sessionID, token),
		StreamTokenExpiresAt: tokenExpiresAt,
		ExpiresAt:            expiresAt,
	}, nil
}

func isActiveSessionUniquenessError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "ux_remote_browser_sessions_active_user_platform") ||
		(strings.Contains(message, "unique") &&
			strings.Contains(message, "remote_browser_sessions") &&
			strings.Contains(message, "user_id") &&
			strings.Contains(message, "platform"))
}
