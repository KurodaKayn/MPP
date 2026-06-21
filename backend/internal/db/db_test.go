package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

type recordingQueryObserver struct {
	observations []QueryObservation
}

func (r *recordingQueryObserver) ObserveQuery(_ context.Context, observation QueryObservation) {
	r.observations = append(r.observations, observation)
}

func TestSyncSchemaKeepsActiveBrowserSessionUniquenessFallback(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, syncSchema(database))

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

func TestSyncSchemaAddsProjectCollabDocumentLink(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, syncSchema(database))

	require.True(t, database.Migrator().HasColumn(&models.Project{}, "collab_document_id"))
	require.True(t, database.Migrator().HasIndex(&models.Project{}, "ux_projects_collab_document"))
}

func TestSyncSchemaAddsWorkspaceTeamModel(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, syncSchema(database))

	require.True(t, database.Migrator().HasTable(&models.Workspace{}))
	require.True(t, database.Migrator().HasTable(&models.WorkspaceMember{}))
	require.True(t, database.Migrator().HasColumn(&models.Project{}, "workspace_id"))
	require.True(t, database.Migrator().HasIndex(&models.Project{}, "idx_projects_workspace_status_created_at"))
	require.True(t, database.Migrator().HasIndex(&models.WorkspaceMember{}, "idx_workspace_members_user_role"))

	owner := models.User{Username: "workspace-owner", Email: "owner@example.com"}
	member := models.User{Username: "workspace-member", Email: "member@example.com"}
	require.NoError(t, database.Create(&owner).Error)
	require.NoError(t, database.Create(&member).Error)

	workspace := models.Workspace{
		OwnerUserID: owner.ID,
		Name:        "Team workspace",
		Slug:        "team-workspace",
	}
	require.NoError(t, database.Create(&workspace).Error)
	require.NotEqual(t, uuid.Nil, workspace.ID)
	require.Equal(t, models.WorkspaceStatusActive, workspace.Status)

	workspaceMember := models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      member.ID,
		Role:        models.WorkspaceRoleMember,
		InvitedBy:   &owner.ID,
	}
	require.NoError(t, database.Create(&workspaceMember).Error)

	project := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Workspace project",
		SourceContent: "content",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, database.Create(&project).Error)

	var loadedProject models.Project
	require.NoError(t, database.Preload("Workspace").First(&loadedProject, "id = ?", project.ID).Error)
	require.NotNil(t, loadedProject.WorkspaceID)
	require.Equal(t, workspace.ID, *loadedProject.WorkspaceID)
	require.Equal(t, workspace.Name, loadedProject.Workspace.Name)
}

func TestSyncSchemaAddsArchiveScanIndexes(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, syncSchema(database))

	require.True(t, database.Migrator().HasIndex(&models.PublishEvent{}, "idx_publish_events_archive_created_id"))
	require.True(t, database.Migrator().HasIndex(&models.ExtensionExecutionEvent{}, "idx_extension_execution_events_archive_created_id"))
	require.True(t, database.Migrator().HasIndex(&models.ProjectActivity{}, "idx_project_activities_archive_created_id"))
	require.True(t, database.Migrator().HasIndex(&models.WorkspaceActivity{}, "idx_workspace_activities_archive_created_id"))
	require.True(t, database.Migrator().HasIndex(&models.RemoteBrowserSession{}, "idx_remote_browser_sessions_archive_status_created_id"))
}

func TestMonthlyPartitionedEventModelsUsePartitionCompatiblePrimaryKeys(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, syncSchema(database))

	for _, tableName := range []string{
		"publish_events",
		"extension_execution_events",
		"project_activities",
		"workspace_activities",
	} {
		primaryKeyColumns := sqlitePrimaryKeyColumns(t, database, tableName)

		require.ElementsMatch(t, []string{"id", "created_at"}, primaryKeyColumns, tableName)
	}
	require.True(t, database.Migrator().HasTable(&models.ExtensionExecutionEventClaim{}))
}

func TestCreateMonthlyPartitionSQLUsesMonthRange(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	sql := createMonthlyPartitionSQL("publish_events", start, start.AddDate(0, 1, 0))

	require.Contains(t, sql, `"publish_events_2026_06"`)
	require.Contains(t, sql, `PARTITION OF "publish_events"`)
	require.Contains(t, sql, `TIMESTAMPTZ '2026-06-01 00:00:00+00'`)
	require.Contains(t, sql, `TIMESTAMPTZ '2026-07-01 00:00:00+00'`)
}

func TestCreateDefaultMonthlyPartitionSQLUsesDefaultPartition(t *testing.T) {
	sql := createDefaultMonthlyPartitionSQL("publish_events")

	require.Contains(t, sql, `"publish_events_default"`)
	require.Contains(t, sql, `PARTITION OF "publish_events"`)
	require.Contains(t, sql, " DEFAULT")
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

func TestReplicaLagMonitorConfigFromEnvDisabledWithoutMaxLag(t *testing.T) {
	clearReplicaLagMonitorEnv(t)

	config, enabled, err := replicaLagMonitorConfigFromEnv()

	require.NoError(t, err)
	require.False(t, enabled)
	require.Zero(t, config)
}

func TestReplicaLagMonitorConfigFromEnvUsesOverrides(t *testing.T) {
	clearReplicaLagMonitorEnv(t)
	t.Setenv(dbReaderMaxLagEnv, "2s")
	t.Setenv(dbReaderLagCheckEnv, "250ms")

	config, enabled, err := replicaLagMonitorConfigFromEnv()

	require.NoError(t, err)
	require.True(t, enabled)
	require.Equal(t, 2*time.Second, config.MaxLag)
	require.Equal(t, 250*time.Millisecond, config.CheckInterval)
}

func TestReplicaLagMonitorConfigFromEnvUsesDefaultCheckInterval(t *testing.T) {
	clearReplicaLagMonitorEnv(t)
	t.Setenv(dbReaderMaxLagEnv, "2s")

	config, enabled, err := replicaLagMonitorConfigFromEnv()

	require.NoError(t, err)
	require.True(t, enabled)
	require.Equal(t, 2*time.Second, config.MaxLag)
	require.Equal(t, defaultLagCheck, config.CheckInterval)
}

func TestReplicaLagMonitorConfigFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "invalid max lag",
			env: map[string]string{
				dbReaderMaxLagEnv: "5",
			},
			wantErr: dbReaderMaxLagEnv,
		},
		{
			name: "invalid check interval",
			env: map[string]string{
				dbReaderMaxLagEnv:   "2s",
				dbReaderLagCheckEnv: "-1s",
			},
			wantErr: dbReaderLagCheckEnv,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearReplicaLagMonitorEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, _, err := replicaLagMonitorConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPostgresDSNFromEnvUsesDefaultSSLMode(t *testing.T) {
	setDatabaseConnectionEnv(t)
	t.Setenv(dbSSLModeEnv, "")
	t.Setenv(dbSSLRootCertEnv, "")

	dsn, err := postgresDSNFromEnv()

	require.NoError(t, err)
	require.Contains(t, dsn, "host='db'")
	require.Contains(t, dsn, "sslmode='disable'")
	require.NotContains(t, dsn, "sslrootcert")
}

func TestPostgresDSNFromEnvDoesNotQuoteRuntimeTimeZone(t *testing.T) {
	setDatabaseConnectionEnv(t)

	dsn, err := postgresDSNFromEnv()

	require.NoError(t, err)
	require.Contains(t, dsn, "TimeZone=Asia/Shanghai")
	require.NotContains(t, dsn, "TimeZone='Asia/Shanghai'")
}

func TestPostgresDSNFromEnvUsesVerifiedTLSSettings(t *testing.T) {
	setDatabaseConnectionEnv(t)
	t.Setenv(dbSSLModeEnv, "verify-full")
	t.Setenv(dbSSLRootCertEnv, "/var/run/secrets/postgres/ca.crt")

	dsn, err := postgresDSNFromEnv()

	require.NoError(t, err)
	require.Contains(t, dsn, "sslmode='verify-full'")
	require.Contains(t, dsn, "sslrootcert='/var/run/secrets/postgres/ca.crt'")
}

func TestPostgresDSNFromEnvRejectsInvalidSSLMode(t *testing.T) {
	setDatabaseConnectionEnv(t)
	t.Setenv(dbSSLModeEnv, "plain")

	dsn, err := postgresDSNFromEnv()

	require.Empty(t, dsn)
	require.Error(t, err)
	require.Contains(t, err.Error(), dbSSLModeEnv)
}

func TestPostgresReadReplicaDSNFromEnvDisabledWithoutHost(t *testing.T) {
	setDatabaseConnectionEnv(t)
	t.Setenv(dbReaderHostEnv, "")

	dsn, enabled, err := postgresReadReplicaDSNFromEnv()

	require.NoError(t, err)
	require.False(t, enabled)
	require.Empty(t, dsn)
}

func TestPostgresReadReplicaDSNFromEnvFallsBackToWriterFields(t *testing.T) {
	setDatabaseConnectionEnv(t)
	t.Setenv(dbSSLModeEnv, "require")
	t.Setenv(dbReaderHostEnv, "reader-db")

	dsn, enabled, err := postgresReadReplicaDSNFromEnv()

	require.NoError(t, err)
	require.True(t, enabled)
	require.Contains(t, dsn, "host='reader-db'")
	require.Contains(t, dsn, "user='postgres'")
	require.Contains(t, dsn, "dbname='poster_db'")
	require.Contains(t, dsn, "port='5432'")
	require.Contains(t, dsn, "sslmode='require'")
}

func TestPostgresReadReplicaDSNFromEnvUsesReaderOverrides(t *testing.T) {
	setDatabaseConnectionEnv(t)
	t.Setenv(dbReaderHostEnv, "reader-db")
	t.Setenv(dbReaderUserEnv, "reader")
	t.Setenv(dbReaderPasswordEnv, "reader-password")
	t.Setenv(dbReaderNameEnv, "reader_db")
	t.Setenv(dbReaderPortEnv, "6543")
	t.Setenv(dbReaderSSLModeEnv, "verify-ca")
	t.Setenv(dbReaderSSLRootEnv, "/reader/ca.crt")

	dsn, enabled, err := postgresReadReplicaDSNFromEnv()

	require.NoError(t, err)
	require.True(t, enabled)
	require.Contains(t, dsn, "host='reader-db'")
	require.Contains(t, dsn, "user='reader'")
	require.Contains(t, dsn, "password='reader-password'")
	require.Contains(t, dsn, "dbname='reader_db'")
	require.Contains(t, dsn, "port='6543'")
	require.Contains(t, dsn, "sslmode='verify-ca'")
	require.Contains(t, dsn, "sslrootcert='/reader/ca.crt'")
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

func TestInstallQueryObserverRecordsSanitizedQueryFacts(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&models.User{}))

	observer := &recordingQueryObserver{}
	require.NoError(t, InstallQueryObserver(database, observer))
	require.NoError(t, InstallQueryObserver(database, observer))

	user := models.User{
		Username:     "observed-user",
		Email:        "observed@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, database.Create(&user).Error)

	var found models.User
	require.NoError(t, database.Where("username = ?", "observed-user").First(&found).Error)

	var queryObservation *QueryObservation
	for i := range observer.observations {
		if observer.observations[i].Operation == "query" {
			queryObservation = &observer.observations[i]
			break
		}
	}

	require.NotNil(t, queryObservation)
	require.Equal(t, "users", queryObservation.Table)
	require.NotEmpty(t, queryObservation.QueryHash)
	require.Positive(t, queryObservation.Duration)
	require.Equal(t, int64(1), queryObservation.RowsAffected)
	require.Contains(t, queryObservation.SQL, "username = ?")
	require.NotContains(t, queryObservation.SQL, "observed-user")
}

func clearConnectionPoolEnv(t *testing.T) {
	t.Helper()
	t.Setenv(dbMaxOpenConnsEnv, "")
	t.Setenv(dbMaxIdleConnsEnv, "")
	t.Setenv(dbConnMaxLifetimeEnv, "")
	t.Setenv(dbConnMaxIdleTimeEnv, "")
}

func clearReplicaLagMonitorEnv(t *testing.T) {
	t.Helper()
	t.Setenv(dbReaderMaxLagEnv, "")
	t.Setenv(dbReaderLagCheckEnv, "")
}

func setDatabaseConnectionEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_USER", "postgres")
	t.Setenv("DB_PASSWORD", "postgres")
	t.Setenv("DB_NAME", "poster_db")
	t.Setenv("DB_PORT", "5432")
}

func sqlitePrimaryKeyColumns(t *testing.T, database *gorm.DB, tableName string) []string {
	t.Helper()

	rows, err := database.Raw("PRAGMA table_info(" + tableName + ")").Rows()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rows.Close())
	}()

	columns := []string{}
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKeyPosition int
		require.NoError(t, rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKeyPosition))
		if primaryKeyPosition > 0 {
			columns = append(columns, name)
		}
	}
	require.NoError(t, rows.Err())
	return columns
}
