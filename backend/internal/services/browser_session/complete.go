package browsersession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
)

func (s *BrowserSessionService) CompleteSession(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*dto.CompleteBrowserSessionResponse, error) {
	var session models.RemoteBrowserSession
	if err := s.strongReadDB(ctx).Where("id = ? AND user_id = ?", id, userID).First(&session).Error; err != nil {
		return nil, ErrSessionNotFound
	}

	if session.Status == models.BrowserSessionStatusConnected {
		return nil, fmt.Errorf("%w: session already completed", ErrSessionNotReady)
	}
	if !isStreamableBrowserSessionStatus(session.Status) {
		return nil, ErrSessionNotReady
	}

	// 1. Transition to capturing
	s.writerDB(ctx).Model(&session).Update("status", models.BrowserSessionStatusCapturing)
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
		s.writerDB(ctx).Model(&session).Updates(map[string]any{
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
		s.writerDB(ctx).Model(&session).Update("status", models.BrowserSessionStatusReady)
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
		return nil, fmt.Errorf("%w: %s", ErrLoginNotDetected, message)
	}

	// 3. Save cookies via CookieStore
	profile := publisher.RemoteAccountProfile{
		Username:       captureResp.Account.Username,
		AvatarURL:      captureResp.Account.AvatarURL,
		PlatformUserID: captureResp.Account.PlatformUserID,
	}
	workspaceID := models.PersonalWorkspaceID(userID)
	if session.WorkspaceID != nil {
		workspaceID = *session.WorkspaceID
	}
	accountID := uuid.Nil
	if session.PlatformAccountID != nil {
		accountID = *session.PlatformAccountID
	}
	authorizedWorkspaceID, err := s.authorizeSessionTarget(ctx, userID, workspaceID, accountID, session.Platform)
	if err != nil {
		s.writerDB(ctx).Model(&session).Update("status", models.BrowserSessionStatusReady)
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
		return nil, err
	}
	workspaceID = authorizedWorkspaceID
	err = s.cookieStore.SaveForAccount(ctx, userID, workspaceID, accountID, session.Platform, captureResp.Cookies, profile)
	if err != nil {
		s.writerDB(ctx).Model(&session).Update("status", models.BrowserSessionStatusReady)
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
	s.invalidateDashboardAccountCache(ctx, workspaceID, session.Platform)

	// 4. Finalize session
	now := time.Now()
	s.writerDB(ctx).Model(&session).Updates(map[string]any{
		"status":       models.BrowserSessionStatusConnected,
		"completed_at": &now,
	})

	// 5. Stop worker
	_ = s.workerClient.StopSession(ctx, session.WorkerSessionRef)
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
	if err := s.strongReadDB(ctx).Where("id = ? AND user_id = ?", id, userID).First(&session).Error; err != nil {
		return ErrSessionNotFound
	}

	if session.WorkerSessionRef != "" {
		_ = s.workerClient.StopSession(ctx, session.WorkerSessionRef)
	}
	_ = s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)

	return s.writerDB(ctx).Model(&session).Updates(map[string]any{
		"status":             models.BrowserSessionStatusExpired,
		"connect_token_hash": "",
	}).Error
}

func (s *BrowserSessionService) invalidateDashboardAccountCache(ctx context.Context, workspaceID uuid.UUID, platform string) {
	if s.dashboardAccountCacheInvalidator == nil || workspaceID == uuid.Nil {
		return
	}
	s.dashboardAccountCacheInvalidator.InvalidateDashboardAccountCache(ctx, workspaceID, platform)
}
