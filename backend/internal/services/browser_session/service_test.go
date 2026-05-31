package browsersession_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBrowserSessionTest(t *testing.T) (*gorm.DB, *browsersession.BrowserSessionService, *publisher.MockBrowserWorkerClient) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate necessary tables
	err = db.AutoMigrate(
		&models.User{},
		&models.PlatformAccount{},
		&models.RemoteBrowserSession{},
	)
	require.NoError(t, err)

	// Create the partial unique index as in production
	err = db.Exec(`
		CREATE UNIQUE INDEX ux_remote_browser_sessions_active_user_platform
		ON remote_browser_sessions (user_id, platform)
		WHERE status IN ('pending', 'ready', 'login_detected', 'capturing')
	`).Error
	require.NoError(t, err)

	worker := publisher.NewMockBrowserWorkerClient()
	t.Cleanup(func() {
		require.NoError(t, worker.Close())
	})
	store := publisher.NewCookieStore(db)
	svc := browsersession.NewBrowserSessionService(db, worker, store)

	return db, svc, worker
}

func setupBrowserSessionRedis(t *testing.T, svc *browsersession.BrowserSessionService) *redis.Client {
	t.Helper()

	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	svc.UseRedis(client)
	return client
}

func setRedisLiveSession(t *testing.T, client *redis.Client, state map[string]interface{}, ttl time.Duration) {
	t.Helper()

	sessionID, ok := state["session_id"].(string)
	require.True(t, ok)
	payload, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, client.Set(context.Background(), "mpp:browser:session:"+sessionID, payload, ttl).Err())
}

func TestBrowserSessionService_FullLifecycle(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	// 1. Start Session
	resp, err := svc.StartSession(context.Background(), userID, platform)
	require.NoError(t, err)
	assert.NotNil(t, resp.SessionID)
	assert.Equal(t, models.BrowserSessionStatusReady, resp.Status)
	assert.Contains(t, resp.StreamURL, resp.SessionID.String())

	streamURL, err := url.Parse(resp.StreamURL)
	require.NoError(t, err)
	assert.Empty(t, streamURL.Query().Get("token"))
	assert.True(t, resp.StreamTokenExpiresAt.After(time.Now()))
	assert.True(t, !resp.StreamTokenExpiresAt.After(resp.ExpiresAt))
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), resp.StreamTokenExpiresAt, 2*time.Second)
	streamToken := streamTokenFromPath(t, streamURL.Path)
	require.NotEmpty(t, streamToken)
	expectedProxyPath := strings.TrimSuffix(strings.TrimPrefix(streamURL.Path, "/"), "/vnc.html") + "/websockify"
	assert.Equal(t, expectedProxyPath, streamURL.Query().Get("path"))

	streamEndpoint, err := svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, false)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(streamEndpoint, "http://127.0.0.1:"))

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, "bad-token", false)
	assert.ErrorIs(t, err, browsersession.ErrInvalidStreamToken)

	// Verify DB state
	var session models.RemoteBrowserSession
	err = db.First(&session, resp.SessionID).Error
	assert.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, session.Status)
	assert.NotEmpty(t, session.WorkerSessionRef)
	assert.WithinDuration(t, resp.StreamTokenExpiresAt, session.ConnectTokenExpiresAt, time.Second)

	// 2. Get Session
	getStatus, err := svc.GetSession(context.Background(), userID, resp.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, getStatus.Status)
	assert.Empty(t, getStatus.StreamURL)

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, true)
	require.NoError(t, err)

	getStatus, err = svc.GetSession(context.Background(), userID, resp.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, getStatus.Status)
	assert.NotEmpty(t, getStatus.StreamURL)

	// 3. Complete Session (Simulate successful login)
	completeResp, err := svc.CompleteSession(context.Background(), userID, resp.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusConnected, completeResp.Status)
	assert.Equal(t, "Mock User", completeResp.Account.Username)

	// Verify PlatformAccount updated
	var account models.PlatformAccount
	err = db.Where("user_id = ? AND platform = ?", userID, platform).First(&account).Error
	assert.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusConnected, account.Status)
	assert.Equal(t, "Mock User", account.Username)

	// 4. Try starting another session (should fail)
	_, err = svc.StartSession(context.Background(), userID, platform)
	// Actually, previous session is now COMPLETED, so we CAN start a new one
	// if we wanted to reconnect. The design doc says "one ACTIVE session".
	// Let's test active session conflict.

	// Create another user for conflict test
	user2ID := uuid.New()
	resp2, err := svc.StartSession(context.Background(), user2ID, platform)
	assert.NoError(t, err)

	_, err = svc.StartSession(context.Background(), user2ID, platform)
	assert.ErrorIs(t, err, browsersession.ErrActiveSessionExists)

	// 5. Cancel the second session
	err = svc.CancelSession(context.Background(), user2ID, resp2.SessionID)
	assert.NoError(t, err)
}

func TestBrowserSessionService_GetStreamEndpointRejectsExpiredDatabaseToken(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSession(context.Background(), userID, platform)
	require.NoError(t, err)
	streamURL, err := url.Parse(resp.StreamURL)
	require.NoError(t, err)
	streamToken := streamTokenFromPath(t, streamURL.Path)

	require.NoError(t, db.Model(&models.RemoteBrowserSession{}).
		Where("id = ?", resp.SessionID).
		Update("connect_token_expires_at", time.Now().Add(-time.Second)).Error)

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, false)
	assert.ErrorIs(t, err, browsersession.ErrInvalidStreamToken)

	status, err := svc.GetSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)
	require.NotEmpty(t, status.StreamURL)
	assert.True(t, status.StreamTokenExpiresAt.After(time.Now()))
	assert.True(t, !status.StreamTokenExpiresAt.After(status.ExpiresAt))

	rotatedURL, err := url.Parse(status.StreamURL)
	require.NoError(t, err)
	rotatedToken := streamTokenFromPath(t, rotatedURL.Path)
	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, rotatedToken, false)
	assert.NoError(t, err)
}

func TestBrowserSessionService_UnsupportedPlatform(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	_, err := svc.StartSession(context.Background(), uuid.New(), "invalid-platform")
	assert.ErrorIs(t, err, browsersession.ErrPlatformNotSupported)
}

func TestBrowserSessionService_StartSessionIgnoresExpiredActiveRows(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	require.NoError(t, db.Create(&models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         platform,
		Status:           models.BrowserSessionStatusReady,
		ConnectTokenHash: "expired-token",
		CreatedAt:        time.Now().Add(-30 * time.Minute),
		ExpiresAt:        time.Now().Add(-15 * time.Minute),
	}).Error)

	resp, err := svc.StartSession(context.Background(), userID, platform)

	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, resp.Status)
	assert.NotEqual(t, uuid.Nil, resp.SessionID)
}

func TestBrowserSessionService_StartSessionExpiresStaleWorkerRows(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	staleSession := models.RemoteBrowserSession{
		UserID:            userID,
		Platform:          platform,
		Status:            models.BrowserSessionStatusReady,
		WorkerSessionRef:  "worker-stale",
		StreamEndpointRef: "http://127.0.0.1:9/stream/worker-stale",
		ConnectTokenHash:  "stale-token",
		CreatedAt:         time.Now().Add(-2 * time.Minute),
		ExpiresAt:         time.Now().Add(13 * time.Minute),
	}
	require.NoError(t, db.Create(&staleSession).Error)

	resp, err := svc.StartSession(context.Background(), userID, platform)

	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, resp.Status)
	assert.NotEqual(t, staleSession.ID, resp.SessionID)

	var savedStaleSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedStaleSession, staleSession.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusExpired, savedStaleSession.Status)
	assert.Equal(t, "worker session is unavailable", savedStaleSession.ErrorMessage)
}

func TestBrowserSessionService_StartSessionPreservesInFlightPendingRows(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	pendingSession := models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         platform,
		Status:           models.BrowserSessionStatusPending,
		ConnectTokenHash: "pending-token",
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(15 * time.Minute),
	}
	require.NoError(t, db.Create(&pendingSession).Error)

	_, err := svc.StartSession(context.Background(), userID, platform)

	assert.ErrorIs(t, err, browsersession.ErrActiveSessionExists)
	var savedPendingSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedPendingSession, pendingSession.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusPending, savedPendingSession.Status)
	assert.Empty(t, savedPendingSession.ErrorMessage)
}

func TestBrowserSessionService_StartSessionExpiresOldPendingRows(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	pendingSession := models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         platform,
		Status:           models.BrowserSessionStatusPending,
		ConnectTokenHash: "pending-token",
		CreatedAt:        time.Now().Add(-3 * time.Minute),
		ExpiresAt:        time.Now().Add(12 * time.Minute),
	}
	require.NoError(t, db.Create(&pendingSession).Error)

	resp, err := svc.StartSession(context.Background(), userID, platform)

	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, resp.Status)
	var savedPendingSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedPendingSession, pendingSession.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusExpired, savedPendingSession.Status)
	assert.Equal(t, "worker session reference is missing", savedPendingSession.ErrorMessage)
}

func TestBrowserSessionService_StartSessionRecoversStaleRedisActiveLock(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	staleSession := models.RemoteBrowserSession{
		UserID:            userID,
		Platform:          platform,
		Status:            models.BrowserSessionStatusReady,
		WorkerSessionRef:  "worker-stale",
		StreamEndpointRef: "http://127.0.0.1:9/stream/worker-stale",
		ConnectTokenHash:  "stale-token",
		CreatedAt:         time.Now().Add(-2 * time.Minute),
		ExpiresAt:         time.Now().Add(13 * time.Minute),
	}
	require.NoError(t, db.Create(&staleSession).Error)
	require.NoError(t, client.Set(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform, staleSession.ID.String(), time.Hour).Err())
	setRedisLiveSession(t, client, map[string]interface{}{
		"session_id":          staleSession.ID.String(),
		"user_id":             userID.String(),
		"platform":            platform,
		"status":              models.BrowserSessionStatusReady,
		"worker_session_ref":  staleSession.WorkerSessionRef,
		"stream_endpoint_ref": staleSession.StreamEndpointRef,
		"created_at":          staleSession.CreatedAt,
		"expires_at":          staleSession.ExpiresAt,
	}, time.Hour)

	resp, err := svc.StartSession(context.Background(), userID, platform)

	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, resp.Status)
	assert.NotEqual(t, staleSession.ID, resp.SessionID)

	activeSessionID, err := client.Get(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform).Result()
	require.NoError(t, err)
	assert.Equal(t, resp.SessionID.String(), activeSessionID)
	assert.Equal(t, int64(0), client.Exists(context.Background(), "mpp:browser:session:"+staleSession.ID.String()).Val())

	var savedStaleSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedStaleSession, staleSession.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusExpired, savedStaleSession.Status)
	assert.Equal(t, "worker heartbeat missing", savedStaleSession.ErrorMessage)
	assert.Empty(t, savedStaleSession.ConnectTokenHash)
}

func TestBrowserSessionService_StartSessionPreservesReachableRedisActiveLock(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSession(context.Background(), userID, platform)
	require.NoError(t, err)

	_, err = svc.StartSession(context.Background(), userID, platform)

	assert.ErrorIs(t, err, browsersession.ErrActiveSessionExists)
	activeSessionID, err := client.Get(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform).Result()
	require.NoError(t, err)
	assert.Equal(t, resp.SessionID.String(), activeSessionID)
}

func TestBrowserSessionService_GetSessionKeepsLiveRedisStateOnTransientWorkerReadFailure(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	userID := uuid.New()
	platform := "douyin"
	workerSessionRef := "worker-temporary-error"
	session := models.RemoteBrowserSession{
		UserID:            userID,
		Platform:          platform,
		Status:            models.BrowserSessionStatusReady,
		WorkerSessionRef:  workerSessionRef,
		StreamEndpointRef: "http://127.0.0.1:9/stream/worker-temporary-error",
		ConnectTokenHash:  "stale-token",
		CreatedAt:         time.Now().Add(-time.Minute),
		ExpiresAt:         time.Now().Add(10 * time.Minute),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, client.Set(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform, session.ID.String(), time.Hour).Err())
	require.NoError(t, client.Set(context.Background(), "mpp:browser:worker-heartbeat:"+workerSessionRef, session.ID.String(), time.Hour).Err())
	setRedisLiveSession(t, client, map[string]interface{}{
		"session_id":          session.ID.String(),
		"user_id":             userID.String(),
		"platform":            platform,
		"status":              models.BrowserSessionStatusReady,
		"worker_session_ref":  workerSessionRef,
		"stream_endpoint_ref": session.StreamEndpointRef,
		"message":             "Waiting for login",
		"created_at":          session.CreatedAt,
		"expires_at":          session.ExpiresAt,
	}, time.Hour)

	resp, err := svc.GetSession(context.Background(), userID, session.ID)

	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, resp.Status)
	assert.Equal(t, "Waiting for login", resp.Message)
	assert.NotEmpty(t, resp.StreamURL)

	var savedSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedSession, session.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusReady, savedSession.Status)
	assert.Equal(t, "stale-token", savedSession.ConnectTokenHash)
	assert.Equal(t, int64(1), client.Exists(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform).Val())
	assert.Equal(t, int64(1), client.Exists(context.Background(), "mpp:browser:session:"+session.ID.String()).Val())
	assert.Equal(t, int64(1), client.Exists(context.Background(), "mpp:browser:worker-heartbeat:"+workerSessionRef).Val())
}

func streamTokenFromPath(t *testing.T, path string) string {
	t.Helper()

	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == "browser-stream" {
			require.GreaterOrEqual(t, len(parts), i+4)
			assert.Equal(t, "vnc.html", parts[i+3])
			return parts[i+2]
		}
	}
	require.Fail(t, "stream token path segment not found", path)
	return ""
}
