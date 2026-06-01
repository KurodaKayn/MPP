package browsersession

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/gorm"
)

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
				now := time.Now()
				if !heartbeatAlive || now.After(state.ExpiresAt) {
					nextStatus := models.BrowserSessionStatusFailed
					message := "worker session is unavailable"
					if !heartbeatAlive {
						message = "worker heartbeat missing"
					}
					if now.After(state.ExpiresAt) {
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
				}
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

	// If expired, check worker if we should update status.
	if time.Now().After(session.ExpiresAt) && !isTerminalBrowserSessionStatus(session.Status) {
		if err := s.CancelSession(ctx, userID, id); err != nil {
			return nil, err
		}
		if s.redisClient != nil {
			return nil, ErrSessionGone
		}
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
	if userID == uuid.Nil {
		return "", ErrSessionForbidden
	}

	var session models.RemoteBrowserSession
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrSessionNotFound
		}
		return "", err
	}
	if session.UserID != userID {
		return "", ErrSessionForbidden
	}

	now := time.Now()
	if now.After(session.ExpiresAt) {
		return "", ErrStreamTokenGone
	}
	if !isStreamableBrowserSessionStatus(session.Status) {
		return "", ErrStreamTokenGone
	}

	tokenHash := hashStreamToken(token)
	if s.redisClient != nil {
		meta, ok, err := s.readRedisStreamToken(ctx, id, tokenHash, consume)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrStreamTokenGone
		}
		if meta.SessionID != id || meta.Platform != session.Platform || meta.Purpose != "stream" {
			return "", ErrInvalidStreamToken
		}
		if meta.UserID != userID {
			return "", ErrSessionForbidden
		}
		if time.Now().After(meta.ExpiresAt) {
			return "", ErrStreamTokenGone
		}
	} else {
		if !streamTokenValidUntil(session).After(now) {
			return "", ErrStreamTokenGone
		}
		if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(session.ConnectTokenHash)) != 1 {
			return "", ErrStreamTokenGone
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
