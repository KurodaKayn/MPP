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
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
)

func setupBrowserSessionTest(t *testing.T) (*gorm.DB, *browsersession.BrowserSessionService, *publisher.MockBrowserWorkerClient) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate necessary tables
	err = db.AutoMigrate(
		&models.User{},
		&models.PlatformAccount{},
		&models.RemoteBrowserSession{},
	)
	require.NoError(t, err)

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

func setRedisLiveSession(t *testing.T, client *redis.Client, state map[string]any, ttl time.Duration) {
	t.Helper()

	sessionID, ok := state["session_id"].(string)
	require.True(t, ok)
	payload, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, client.Set(context.Background(), "mpp:browser:session:"+sessionID, payload, ttl).Err())
}

type dashboardAccountCacheInvalidation struct {
	workspaceID uuid.UUID
	platform    string
}

type fakeDashboardAccountCacheInvalidator struct {
	calls []dashboardAccountCacheInvalidation
}

func (f *fakeDashboardAccountCacheInvalidator) InvalidateDashboardAccountCache(_ context.Context, workspaceID uuid.UUID, platform string) {
	f.calls = append(f.calls, dashboardAccountCacheInvalidation{
		workspaceID: workspaceID,
		platform:    platform,
	})
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
	assert.False(t, resp.StreamTokenExpiresAt.After(resp.ExpiresAt))
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), resp.StreamTokenExpiresAt, 2*time.Second)
	streamToken := streamTokenFromPath(t, streamURL.Path)
	require.NotEmpty(t, streamToken)
	expectedProxyPath := strings.TrimSuffix(strings.TrimPrefix(streamURL.Path, "/"), "/vnc.html") + "/websockify"
	assert.Equal(t, expectedProxyPath, streamURL.Query().Get("path"))

	streamEndpoint, err := svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, false)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(streamEndpoint, "http://127.0.0.1:"))

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, "bad-token", false)
	require.ErrorIs(t, err, browsersession.ErrInvalidStreamToken)

	// Verify DB state
	var session models.RemoteBrowserSession
	err = db.First(&session, resp.SessionID).Error
	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, session.Status)
	assert.NotEmpty(t, session.WorkerSessionRef)
	assert.WithinDuration(t, resp.StreamTokenExpiresAt, session.ConnectTokenExpiresAt, time.Second)

	// 2. Get Session
	getStatus, err := svc.GetSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, getStatus.Status)
	assert.Empty(t, getStatus.StreamURL)

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, true)
	require.NoError(t, err)

	getStatus, err = svc.GetSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, getStatus.Status)
	assert.NotEmpty(t, getStatus.StreamURL)

	// 3. Complete Session (Simulate successful login)
	completeResp, err := svc.CompleteSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusConnected, completeResp.Status)
	assert.Equal(t, "Mock User", completeResp.Account.Username)

	// Verify PlatformAccount updated
	var account models.PlatformAccount
	err = db.Where("user_id = ? AND platform = ?", userID, platform).First(&account).Error
	require.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusConnected, account.Status)
	assert.Equal(t, "Mock User", account.Username)
	assert.Equal(t, "mock-platform-user", account.PlatformUserID)

	// 4. Test active session conflict with a different user.
	user2ID := uuid.New()
	resp2, err := svc.StartSession(context.Background(), user2ID, platform)
	require.NoError(t, err)

	_, err = svc.StartSession(context.Background(), user2ID, platform)
	require.ErrorIs(t, err, browsersession.ErrActiveSessionExists)

	// 5. Cancel the second session
	err = svc.CancelSession(context.Background(), user2ID, resp2.SessionID)
	require.NoError(t, err)
}

func TestBrowserSessionService_CompleteSessionInvalidatesDashboardAccountCache(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	workspaceID := uuid.New()
	invalidator := &fakeDashboardAccountCacheInvalidator{}
	svc.UseDashboardAccountCacheInvalidator(invalidator)
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSessionForWorkspace(context.Background(), userID, "workspace", workspaceID, uuid.Nil, "douyin")
	require.NoError(t, err)

	_, err = svc.CompleteSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)

	require.Equal(t, []dashboardAccountCacheInvalidation{
		{
			workspaceID: workspaceID,
			platform:    "douyin",
		},
	}, invalidator.calls)
}

func TestBrowserSessionService_CompleteSessionInvalidatesPersonalWorkspaceCacheWhenSessionWorkspaceMissing(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	invalidator := &fakeDashboardAccountCacheInvalidator{}
	svc.UseDashboardAccountCacheInvalidator(invalidator)
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSession(context.Background(), userID, "douyin")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.RemoteBrowserSession{}).
		Where("id = ?", resp.SessionID).
		Update("workspace_id", nil).Error)

	_, err = svc.CompleteSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)

	require.Equal(t, []dashboardAccountCacheInvalidation{
		{
			workspaceID: models.PersonalWorkspaceID(userID),
			platform:    "douyin",
		},
	}, invalidator.calls)
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
	require.ErrorIs(t, err, browsersession.ErrStreamTokenGone)

	status, err := svc.GetSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)
	require.NotEmpty(t, status.StreamURL)
	assert.True(t, status.StreamTokenExpiresAt.After(time.Now()))
	assert.False(t, status.StreamTokenExpiresAt.After(status.ExpiresAt))

	rotatedURL, err := url.Parse(status.StreamURL)
	require.NoError(t, err)
	rotatedToken := streamTokenFromPath(t, rotatedURL.Path)
	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, rotatedToken, false)
	require.NoError(t, err)
}

func TestBrowserSessionService_GetSessionPreservesTerminalAuditStatusAfterExpiry(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	now := time.Now()
	completedAt := now.Add(-20 * time.Minute)
	session := models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         "douyin",
		Status:           models.BrowserSessionStatusConnected,
		ConnectTokenHash: "",
		CreatedAt:        now.Add(-30 * time.Minute),
		ExpiresAt:        now.Add(-15 * time.Minute),
		CompletedAt:      &completedAt,
	}
	require.NoError(t, db.Create(&session).Error)

	resp, err := svc.GetSession(context.Background(), userID, session.ID)

	require.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusConnected, resp.Status)

	var savedSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedSession, session.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusConnected, savedSession.Status)
}

func TestBrowserSessionService_GetSessionReturnsGoneForExpiredRedisSession(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	userID := uuid.New()
	platform := "douyin"
	now := time.Now()
	session := models.RemoteBrowserSession{
		UserID:            userID,
		Platform:          platform,
		Status:            models.BrowserSessionStatusReady,
		WorkerSessionRef:  "worker-expired",
		StreamEndpointRef: "http://127.0.0.1:9/stream/worker-expired",
		ConnectTokenHash:  "expired-token",
		CreatedAt:         now.Add(-30 * time.Minute),
		ExpiresAt:         now.Add(-15 * time.Minute),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, client.Set(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform, session.ID.String(), time.Hour).Err())

	_, err := svc.GetSession(context.Background(), userID, session.ID)

	require.ErrorIs(t, err, browsersession.ErrSessionGone)
	assert.Equal(t, int64(0), client.Exists(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform).Val())

	var savedSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedSession, session.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusExpired, savedSession.Status)
}

func TestBrowserSessionService_RedisStreamTokenIsConsumedOnce(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	setupBrowserSessionRedis(t, svc)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSession(context.Background(), userID, platform)
	require.NoError(t, err)
	streamURL, err := url.Parse(resp.StreamURL)
	require.NoError(t, err)
	streamToken := streamTokenFromPath(t, streamURL.Path)

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, false)
	require.NoError(t, err)
	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, true)
	require.NoError(t, err)
	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, streamToken, false)
	require.ErrorIs(t, err, browsersession.ErrStreamTokenGone)

	status, err := svc.GetSession(context.Background(), userID, resp.SessionID)
	require.NoError(t, err)
	require.NotEmpty(t, status.StreamURL)

	rotatedURL, err := url.Parse(status.StreamURL)
	require.NoError(t, err)
	rotatedToken := streamTokenFromPath(t, rotatedURL.Path)
	assert.NotEqual(t, streamToken, rotatedToken)

	_, err = svc.GetStreamEndpoint(context.Background(), userID, resp.SessionID, rotatedToken, false)
	require.NoError(t, err)
}

func TestBrowserSessionService_CancelSessionDeletesAllRedisStreamTokens(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSession(context.Background(), userID, platform)
	require.NoError(t, err)
	strayTokenKey := "mpp:browser:stream-token:" + resp.SessionID.String() + ":stray-token-hash"
	require.NoError(t, client.Set(context.Background(), strayTokenKey, "{}", time.Hour).Err())

	require.NoError(t, svc.CancelSession(context.Background(), userID, resp.SessionID))

	tokenKeys, err := client.Keys(context.Background(), "mpp:browser:stream-token:"+resp.SessionID.String()+":*").Result()
	require.NoError(t, err)
	assert.Empty(t, tokenKeys)
	assert.Equal(t, int64(0), client.Exists(context.Background(), "mpp:browser:stream-current:"+resp.SessionID.String()).Val())
}

func TestBrowserSessionService_UnsupportedPlatform(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	_, err := svc.StartSession(context.Background(), uuid.New(), "invalid-platform")
	require.ErrorIs(t, err, browsersession.ErrPlatformNotSupported)
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

	require.ErrorIs(t, err, browsersession.ErrActiveSessionExists)
	var savedPendingSession models.RemoteBrowserSession
	require.NoError(t, db.First(&savedPendingSession, pendingSession.ID).Error)
	assert.Equal(t, models.BrowserSessionStatusPending, savedPendingSession.Status)
	assert.Empty(t, savedPendingSession.ErrorMessage)
}

func TestBrowserSessionService_StartSessionMapsConcurrentDatabaseGuardToConflict(t *testing.T) {
	db, svc, _ := setupBrowserSessionTest(t)
	userID := uuid.New()
	platform := "douyin"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	insertedConcurrentSession := false
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:browser-session-race", func(tx *gorm.DB) {
		session, ok := tx.Statement.Dest.(*models.RemoteBrowserSession)
		if !ok || insertedConcurrentSession || session.UserID != userID || session.Platform != platform {
			return
		}
		insertedConcurrentSession = true
		now := time.Now()
		conflictingSession := models.RemoteBrowserSession{
			UserID:           userID,
			Platform:         platform,
			Status:           models.BrowserSessionStatusReady,
			ConnectTokenHash: "concurrent-token",
			CreatedAt:        now,
			ExpiresAt:        now.Add(15 * time.Minute),
		}
		_ = tx.Session(&gorm.Session{NewDB: true}).Create(&conflictingSession).Error
	}))

	_, err := svc.StartSession(context.Background(), userID, platform)

	require.True(t, insertedConcurrentSession)
	require.ErrorIs(t, err, browsersession.ErrActiveSessionExists)
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
	setRedisLiveSession(t, client, map[string]any{
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

	require.ErrorIs(t, err, browsersession.ErrActiveSessionExists)
	activeSessionID, err := client.Get(context.Background(), "mpp:browser:active:"+userID.String()+":"+platform).Result()
	require.NoError(t, err)
	assert.Equal(t, resp.SessionID.String(), activeSessionID)
}

func TestBrowserSessionService_StartSessionEnforcesUserConcurrencyQuota(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	svc.UseQuotaConfig(browsersession.BrowserSessionQuotaConfig{
		UserConcurrencyLimit:   1,
		TenantConcurrencyLimit: 10,
	})
	userID := uuid.New()
	tenantID := "tenant-acme"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSessionForTenant(context.Background(), userID, tenantID, "douyin")
	require.NoError(t, err)

	_, err = svc.StartSessionForTenant(context.Background(), userID, tenantID, "zhihu")
	require.ErrorIs(t, err, browsersession.ErrUserQuotaExceeded)
	assert.Equal(t, int64(1), client.ZCard(context.Background(), "mpp:browser:quota:user:"+userID.String()).Val())

	require.NoError(t, svc.CancelSession(context.Background(), userID, resp.SessionID))
	assert.Equal(t, int64(0), client.ZCard(context.Background(), "mpp:browser:quota:user:"+userID.String()).Val())

	resp, err = svc.StartSessionForTenant(context.Background(), userID, tenantID, "zhihu")
	require.NoError(t, err)
	require.NoError(t, svc.CancelSession(context.Background(), userID, resp.SessionID))
}

func TestBrowserSessionService_StartSessionEnforcesTenantConcurrencyQuota(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	client := setupBrowserSessionRedis(t, svc)
	svc.UseQuotaConfig(browsersession.BrowserSessionQuotaConfig{
		UserConcurrencyLimit:   10,
		TenantConcurrencyLimit: 1,
	})
	userID := uuid.New()
	otherUserID := uuid.New()
	tenantID := "tenant-acme"
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	resp, err := svc.StartSessionForTenant(context.Background(), userID, tenantID, "douyin")
	require.NoError(t, err)

	_, err = svc.StartSessionForTenant(context.Background(), otherUserID, tenantID, "zhihu")
	require.ErrorIs(t, err, browsersession.ErrTenantQuotaExceeded)
	assert.Equal(t, int64(1), client.ZCard(context.Background(), "mpp:browser:quota:tenant:"+tenantID).Val())

	require.NoError(t, svc.CancelSession(context.Background(), userID, resp.SessionID))
	assert.Equal(t, int64(0), client.ZCard(context.Background(), "mpp:browser:quota:tenant:"+tenantID).Val())

	resp, err = svc.StartSessionForTenant(context.Background(), otherUserID, tenantID, "zhihu")
	require.NoError(t, err)
	require.NoError(t, svc.CancelSession(context.Background(), otherUserID, resp.SessionID))
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
	setRedisLiveSession(t, client, map[string]any{
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
		if part == "stream" {
			require.GreaterOrEqual(t, len(parts), i+3)
			assert.Equal(t, "vnc.html", parts[i+2])
			return parts[i+1]
		}
	}
	require.Fail(t, "stream token path segment not found", path)
	return ""
}
