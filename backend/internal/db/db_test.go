package db

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type recordingQueryObserver struct {
	observations []QueryObservation
}

func (r *recordingQueryObserver) ObserveQuery(_ context.Context, observation QueryObservation) {
	r.observations = append(r.observations, observation)
}

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

func TestMigrateAddsProjectCollabDocumentLink(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrate(database))

	require.True(t, database.Migrator().HasColumn(&models.Project{}, "collab_document_id"))
	require.True(t, database.Migrator().HasIndex(&models.Project{}, "ux_projects_collab_document"))
}

func TestMigrateAddsWorkspaceTeamModel(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrate(database))

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

func TestMigrateBackfillsPersonalWorkspaces(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, database.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL UNIQUE,
		is_email_verified BOOLEAN NOT NULL DEFAULT 0,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, database.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		collab_document_id TEXT UNIQUE,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	ownerID := uuid.New()
	emptyUserID := uuid.New()
	projectID := uuid.New()
	require.NoError(t, database.Exec(
		`INSERT INTO users (id, username, email, password_hash, role) VALUES (?, ?, ?, ?, ?)`,
		ownerID.String(),
		"owner",
		"owner@example.com",
		"hash",
		"user",
	).Error)
	require.NoError(t, database.Exec(
		`INSERT INTO users (id, username, email, password_hash, role) VALUES (?, ?, ?, ?, ?)`,
		emptyUserID.String(),
		"empty-user",
		"empty-user@example.com",
		"hash",
		"user",
	).Error)
	require.NoError(t, database.Exec(
		`INSERT INTO projects (id, user_id, title, source_content, status) VALUES (?, ?, ?, ?, ?)`,
		projectID.String(),
		ownerID.String(),
		"Legacy project",
		"content",
		models.ProjectStatusDraft,
	).Error)

	require.NoError(t, migrate(database))

	ownerWorkspaceID := personalWorkspaceID(ownerID)
	emptyUserWorkspaceID := personalWorkspaceID(emptyUserID)

	var workspaceCount int64
	require.NoError(t, database.Model(&models.Workspace{}).Count(&workspaceCount).Error)
	require.Equal(t, int64(2), workspaceCount)

	var ownerWorkspace models.Workspace
	require.NoError(t, database.First(&ownerWorkspace, "id = ?", ownerWorkspaceID).Error)
	require.Equal(t, ownerID, ownerWorkspace.OwnerUserID)
	require.Equal(t, personalWorkspaceName, ownerWorkspace.Name)
	require.Equal(t, personalWorkspaceSlug(ownerID), ownerWorkspace.Slug)
	require.Equal(t, models.WorkspaceStatusActive, ownerWorkspace.Status)

	var emptyUserWorkspace models.Workspace
	require.NoError(t, database.First(&emptyUserWorkspace, "id = ?", emptyUserWorkspaceID).Error)
	require.Equal(t, emptyUserID, emptyUserWorkspace.OwnerUserID)

	var ownerMembership models.WorkspaceMember
	require.NoError(t, database.First(&ownerMembership, "workspace_id = ? AND user_id = ?", ownerWorkspaceID, ownerID).Error)
	require.Equal(t, models.WorkspaceRoleOwner, ownerMembership.Role)
	require.NotNil(t, ownerMembership.JoinedAt)

	var project models.Project
	require.NoError(t, database.First(&project, "id = ?", projectID).Error)
	require.NotNil(t, project.WorkspaceID)
	require.Equal(t, ownerWorkspaceID, *project.WorkspaceID)

	require.NoError(t, migrate(database))
	require.NoError(t, database.Model(&models.Workspace{}).Count(&workspaceCount).Error)
	require.Equal(t, int64(2), workspaceCount)

	var membershipCount int64
	require.NoError(t, database.Model(&models.WorkspaceMember{}).Count(&membershipCount).Error)
	require.Equal(t, int64(2), membershipCount)

	var reloadedProject models.Project
	require.NoError(t, database.First(&reloadedProject, "id = ?", projectID).Error)
	require.NotNil(t, reloadedProject.WorkspaceID)
	require.Equal(t, ownerWorkspaceID, *reloadedProject.WorkspaceID)
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
