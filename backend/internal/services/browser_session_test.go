package services_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBrowserSessionTest(t *testing.T) (*gorm.DB, *services.BrowserSessionService, *publisher.MockBrowserWorkerClient) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate necessary tables
	err = db.AutoMigrate(
		&models.User{},
		&models.PlatformAccount{},
		&models.RemoteBrowserSession{},
	)
	require.NoError(t, err)

	worker := publisher.NewMockBrowserWorkerClient()
	store := publisher.NewCookieStore(db)
	svc := services.NewBrowserSessionService(db, worker, store)

	return db, svc, worker
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

	// Verify DB state
	var session models.RemoteBrowserSession
	err = db.First(&session, resp.SessionID).Error
	assert.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, session.Status)
	assert.NotEmpty(t, session.WorkerSessionRef)

	// 2. Get Session
	getStatus, err := svc.GetSession(context.Background(), userID, resp.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, models.BrowserSessionStatusReady, getStatus.Status)

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
	assert.ErrorIs(t, err, services.ErrActiveSessionExists)

	// 5. Cancel the second session
	err = svc.CancelSession(context.Background(), user2ID, resp2.SessionID)
	assert.NoError(t, err)
}

func TestBrowserSessionService_UnsupportedPlatform(t *testing.T) {
	_, svc, _ := setupBrowserSessionTest(t)
	_, err := svc.StartSession(context.Background(), uuid.New(), "invalid-platform")
	assert.ErrorIs(t, err, services.ErrPlatformNotSupported)
}
