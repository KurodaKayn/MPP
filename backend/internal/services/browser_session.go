package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"gorm.io/gorm"
)

var (
	ErrActiveSessionExists  = errors.New("an active session already exists for this platform")
	ErrPlatformNotSupported = errors.New("platform does not support remote browser sessions")
	ErrSessionNotFound      = errors.New("session not found")
	ErrInvalidStreamToken   = errors.New("invalid or expired stream token")
)

type BrowserSessionService struct {
	db           *gorm.DB
	workerClient publisher.BrowserWorkerClient
	cookieStore  *publisher.CookieStore
	adapters     map[string]publisher.RemoteBrowserPlatformAdapter
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
	return s
}

func (s *BrowserSessionService) RegisterAdapter(a publisher.RemoteBrowserPlatformAdapter) {
	s.adapters[a.Platform()] = a
}

func (s *BrowserSessionService) StartSession(ctx context.Context, userID uuid.UUID, platform string) (*dto.StartBrowserSessionResponse, error) {
	adapter, ok := s.adapters[platform]
	if !ok {
		return nil, ErrPlatformNotSupported
	}

	// 1. Check for active sessions
	var count int64
	s.db.Model(&models.RemoteBrowserSession{}).
		Where("user_id = ? AND platform = ? AND expires_at > ? AND status IN ?", userID, platform, time.Now(), []string{
			models.BrowserSessionStatusPending,
			models.BrowserSessionStatusReady,
			models.BrowserSessionStatusLoginDetected,
			models.BrowserSessionStatusCapturing,
		}).Count(&count)

	if count > 0 {
		return nil, ErrActiveSessionExists
	}

	// 2. Generate stream token
	token, tokenHash, err := generateStreamToken()
	if err != nil {
		return nil, err
	}

	// 3. Create session in DB
	sessionID := uuid.New()
	expiresAt := time.Now().Add(15 * time.Minute)
	session := &models.RemoteBrowserSession{
		ID:               sessionID,
		UserID:           userID,
		Platform:         platform,
		Status:           models.BrowserSessionStatusPending,
		ConnectTokenHash: tokenHash,
		CreatedAt:        time.Now(),
		ExpiresAt:        expiresAt,
	}

	if err := s.db.Create(session).Error; err != nil {
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
		return nil, err
	}

	streamURL := fmt.Sprintf("/api/user/dashboard/browser-sessions/%s/stream?token=%s", sessionID, token)

	return &dto.StartBrowserSessionResponse{
		SessionID:            sessionID,
		Status:               models.BrowserSessionStatusReady,
		StreamURL:            streamURL,
		StreamTokenExpiresAt: time.Now().Add(5 * time.Minute),
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

	// Rotated token logic could go here if needed for reconnect
	return resp, nil
}

func (s *BrowserSessionService) GetStreamEndpoint(ctx context.Context, userID uuid.UUID, id uuid.UUID, token string) (string, error) {
	if token == "" {
		return "", ErrInvalidStreamToken
	}

	var session models.RemoteBrowserSession
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrSessionNotFound
		}
		return "", err
	}

	if time.Now().After(session.ExpiresAt) {
		return "", ErrInvalidStreamToken
	}

	tokenHash := hashStreamToken(token)
	if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(session.ConnectTokenHash)) != 1 {
		return "", ErrInvalidStreamToken
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

	// 1. Transition to capturing
	s.db.Model(&session).Update("status", models.BrowserSessionStatusCapturing)

	// 2. Ask worker to capture
	captureResp, err := s.workerClient.CaptureSession(ctx, session.WorkerSessionRef)
	if err != nil {
		s.db.Model(&session).Updates(map[string]interface{}{
			"status":        models.BrowserSessionStatusReady,
			"error_message": err.Error(),
		})
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	if captureResp.Status != "login_detected" {
		s.db.Model(&session).Update("status", models.BrowserSessionStatusReady)
		return nil, fmt.Errorf("login not detected yet")
	}

	// 3. Save cookies via CookieStore
	profile := publisher.RemoteAccountProfile{
		Username:  captureResp.Account.Username,
		AvatarURL: captureResp.Account.AvatarURL,
	}
	err = s.cookieStore.Save(ctx, userID, session.Platform, captureResp.Cookies, profile)
	if err != nil {
		s.db.Model(&session).Update("status", models.BrowserSessionStatusReady)
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

	return s.db.Model(&session).Updates(map[string]interface{}{
		"status": models.BrowserSessionStatusExpired,
	}).Error
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
