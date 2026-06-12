package readmodel_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	readmodel "github.com/kurodakayn/mpp-backend/internal/services/readmodel"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestRefreshProjectUpsertsProjectListSummaryAndWorkspaceStats(t *testing.T) {
	db := testsupport.SetupTestDB()
	service := readmodel.NewService(db)
	userID := uuid.New()
	workspaceID := uuid.New()
	require.NoError(t, db.Create(&models.User{ID: userID, Username: "writer", Email: "writer@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.Workspace{ID: workspaceID, OwnerUserID: userID, Name: "Team", Status: models.WorkspaceStatusActive}).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{WorkspaceID: workspaceID, UserID: userID, Role: models.WorkspaceRoleOwner}).Error)

	project := models.Project{
		ID:            uuid.New(),
		UserID:        userID,
		WorkspaceID:   &workspaceID,
		Title:         "Launch plan",
		SourceContent: "<p>draft</p>",
		Status:        models.ProjectStatusReady,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:           uuid.New(),
		ProjectID:    project.ID,
		Platform:     "wechat",
		Enabled:      true,
		Status:       models.PublicationStatusPublished,
		DraftStatus:  models.PublicationDraftStatusReady,
		ReviewStatus: models.PublicationReviewStatusDraft,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:           uuid.New(),
		ProjectID:    project.ID,
		Platform:     "x",
		Enabled:      true,
		Status:       models.PublicationStatusFailed,
		DraftStatus:  models.PublicationDraftStatusStale,
		ReviewStatus: models.PublicationReviewStatusDraft,
	}).Error)

	require.NoError(t, service.RefreshProject(project.ID))

	var summary models.ProjectListSummary
	require.NoError(t, db.First(&summary, "project_id = ?", project.ID).Error)
	require.Equal(t, project.Title, summary.Title)
	require.Equal(t, workspaceID, summary.WorkspaceID)
	var publications []dto.PublicationSummary
	require.NoError(t, json.Unmarshal(summary.Publications, &publications))
	require.Len(t, publications, 2)
	require.Equal(t, "wechat", publications[0].Platform)
	require.Equal(t, models.PublicationStatusPublished, publications[0].Status)

	var stats models.WorkspaceDashboardStats
	require.NoError(t, db.First(&stats, "workspace_id = ?", workspaceID).Error)
	require.Equal(t, int64(1), stats.TotalProjects)
	require.Equal(t, int64(1), stats.TotalPublishedPublications)
	require.Equal(t, int64(1), stats.TotalFailedPublications)
	require.Equal(t, int64(1), stats.TotalMembers)
}

func TestRefreshWorkspaceAsyncRecomputesStats(t *testing.T) {
	db := testsupport.SetupTestDB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	service := readmodel.NewService(db)
	userID := uuid.New()
	workspaceID := uuid.New()
	require.NoError(t, db.Create(&models.User{ID: userID, Username: "member", Email: "member@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.Workspace{ID: workspaceID, OwnerUserID: userID, Name: "Team", Status: models.WorkspaceStatusActive}).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{WorkspaceID: workspaceID, UserID: userID, Role: models.WorkspaceRoleOwner}).Error)

	service.RefreshWorkspaceAsync(context.Background(), workspaceID)

	require.Eventually(t, func() bool {
		var stats models.WorkspaceDashboardStats
		if err := db.First(&stats, "workspace_id = ?", workspaceID).Error; err != nil {
			return false
		}
		return stats.TotalMembers == 1
	}, time.Second, 10*time.Millisecond)
}
