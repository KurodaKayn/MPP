package db

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateKeepsActiveBrowserSessionUniquenessFallback(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrate(database))

	userID := uuid.New()
	now := time.Now()
	activeSession := models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         "douyin",
		Status:           models.BrowserSessionStatusReady,
		ConnectTokenHash: "active-token",
		CreatedAt:        now,
		ExpiresAt:        now.Add(time.Hour),
	}
	require.NoError(t, database.Create(&activeSession).Error)

	duplicateActiveSession := models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         "douyin",
		Status:           models.BrowserSessionStatusPending,
		ConnectTokenHash: "duplicate-token",
		CreatedAt:        now,
		ExpiresAt:        now.Add(time.Hour),
	}
	require.Error(t, database.Create(&duplicateActiveSession).Error)

	expiredSession := models.RemoteBrowserSession{
		UserID:           userID,
		Platform:         "douyin",
		Status:           models.BrowserSessionStatusExpired,
		ConnectTokenHash: "expired-token",
		CreatedAt:        now,
		ExpiresAt:        now.Add(-time.Hour),
	}
	require.NoError(t, database.Create(&expiredSession).Error)
}
