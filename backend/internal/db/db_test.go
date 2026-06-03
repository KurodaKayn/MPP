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

func TestConnectionPoolConfigFromEnvUsesDefaults(t *testing.T) {
	clearConnectionPoolEnv(t)

	config, err := connectionPoolConfigFromEnv()

	require.NoError(t, err)
	require.Equal(t, defaultMaxOpenConns, config.MaxOpenConns)
	require.Equal(t, defaultMaxIdleConns, config.MaxIdleConns)
	require.Equal(t, defaultConnMaxLife, config.ConnMaxLifetime)
	require.Equal(t, defaultConnMaxIdle, config.ConnMaxIdleTime)
}

func TestConnectionPoolConfigFromEnvUsesOverrides(t *testing.T) {
	t.Setenv(dbMaxOpenConnsEnv, "24")
	t.Setenv(dbMaxIdleConnsEnv, "8")
	t.Setenv(dbConnMaxLifetimeEnv, "45m")
	t.Setenv(dbConnMaxIdleTimeEnv, "90s")

	config, err := connectionPoolConfigFromEnv()

	require.NoError(t, err)
	require.Equal(t, 24, config.MaxOpenConns)
	require.Equal(t, 8, config.MaxIdleConns)
	require.Equal(t, 45*time.Minute, config.ConnMaxLifetime)
	require.Equal(t, 90*time.Second, config.ConnMaxIdleTime)
}

func TestConnectionPoolConfigFromEnvAllowsLowerMaxOpenWithoutIdleOverride(t *testing.T) {
	clearConnectionPoolEnv(t)
	t.Setenv(dbMaxOpenConnsEnv, "2")

	config, err := connectionPoolConfigFromEnv()

	require.NoError(t, err)
	require.Equal(t, 2, config.MaxOpenConns)
	require.Equal(t, defaultMaxIdleConns, config.MaxIdleConns)
}

func TestConnectionPoolConfigFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "negative max open conns",
			env: map[string]string{
				dbMaxOpenConnsEnv: "-1",
			},
			wantErr: dbMaxOpenConnsEnv,
		},
		{
			name: "invalid lifetime",
			env: map[string]string{
				dbConnMaxLifetimeEnv: "30",
			},
			wantErr: dbConnMaxLifetimeEnv,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConnectionPoolEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := connectionPoolConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestConfigureConnectionPoolAppliesMaxOpenConns(t *testing.T) {
	clearConnectionPoolEnv(t)
	t.Setenv(dbMaxOpenConnsEnv, "3")
	t.Setenv(dbMaxIdleConnsEnv, "2")

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, configureConnectionPool(database))

	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.Equal(t, 3, sqlDB.Stats().MaxOpenConnections)
}

func clearConnectionPoolEnv(t *testing.T) {
	t.Helper()
	t.Setenv(dbMaxOpenConnsEnv, "")
	t.Setenv(dbMaxIdleConnsEnv, "")
	t.Setenv(dbConnMaxLifetimeEnv, "")
	t.Setenv(dbConnMaxIdleTimeEnv, "")
}
