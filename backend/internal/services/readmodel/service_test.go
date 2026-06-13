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

func TestRebuildDashboardReplaysFactsAndRemovesOrphanReadModels(t *testing.T) {
	db := testsupport.SetupTestDB()
	service := readmodel.NewService(db)

	userID := uuid.New()
	workspaceID := uuid.New()
	projectID := uuid.New()
	collabDocumentID := uuid.New()
	templateID := uuid.New()
	brandProfileID := uuid.New()
	now := time.Now().UTC()

	require.NoError(t, db.Create(&models.User{ID: userID, Username: "rebuild-owner", Email: "rebuild-owner@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.Workspace{ID: workspaceID, OwnerUserID: userID, Name: "Rebuild", Status: models.WorkspaceStatusActive, CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{WorkspaceID: workspaceID, UserID: userID, Role: models.WorkspaceRoleOwner, CreatedAt: now}).Error)
	require.NoError(t, db.Create(&models.Project{
		ID:               projectID,
		UserID:           userID,
		WorkspaceID:      &workspaceID,
		CollabDocumentID: &collabDocumentID,
		TemplateID:       &templateID,
		BrandProfileID:   &brandProfileID,
		Title:            "Rebuilt project",
		SourceContent:    "<p>fact</p>",
		Status:           models.ProjectStatusReady,
		CreatedAt:        now.Add(-time.Hour),
		UpdatedAt:        now,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:           uuid.New(),
		ProjectID:    projectID,
		Platform:     "wechat",
		Enabled:      true,
		Status:       models.PublicationStatusPublished,
		DraftStatus:  models.PublicationDraftStatusReady,
		ReviewStatus: models.PublicationReviewStatusDraft,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:           uuid.New(),
		ProjectID:    projectID,
		Platform:     "x",
		Enabled:      true,
		Status:       models.PublicationStatusFailed,
		DraftStatus:  models.PublicationDraftStatusStale,
		ReviewStatus: models.PublicationReviewStatusDraft,
	}).Error)

	orphanProjectID := uuid.New()
	orphanWorkspaceID := uuid.New()
	require.NoError(t, db.Create(&models.ProjectListSummary{
		ProjectID:    orphanProjectID,
		UserID:       userID,
		WorkspaceID:  workspaceID,
		Title:        "orphan",
		Status:       models.ProjectStatusDraft,
		Publications: []byte(`[]`),
		CreatedAt:    now,
		UpdatedAt:    now,
		RefreshedAt:  now,
	}).Error)
	require.NoError(t, db.Create(&models.WorkspaceDashboardStats{
		WorkspaceID:                orphanWorkspaceID,
		TotalProjects:              99,
		TotalPublishedPublications: 99,
		TotalFailedPublications:    99,
		TotalMembers:               99,
		RefreshedAt:                now,
	}).Error)

	result, err := service.RebuildDashboard()
	require.NoError(t, err)
	require.Equal(t, int64(1), result.ProjectsRefreshed)
	require.Equal(t, int64(1), result.WorkspacesRefreshed)
	require.Equal(t, int64(1), result.OrphanProjectSummariesDeleted)
	require.Equal(t, int64(1), result.OrphanWorkspaceStatsDeleted)

	var summary models.ProjectListSummary
	require.NoError(t, db.First(&summary, "project_id = ?", projectID).Error)
	require.Equal(t, "Rebuilt project", summary.Title)
	require.Equal(t, workspaceID, summary.WorkspaceID)
	require.Equal(t, &collabDocumentID, summary.CollabDocumentID)
	require.Equal(t, &templateID, summary.TemplateID)
	require.Equal(t, &brandProfileID, summary.BrandProfileID)
	var publications []dto.PublicationSummary
	require.NoError(t, json.Unmarshal(summary.Publications, &publications))
	require.Len(t, publications, 2)

	var orphanSummaries int64
	require.NoError(t, db.Model(&models.ProjectListSummary{}).Where("project_id = ?", orphanProjectID).Count(&orphanSummaries).Error)
	require.Zero(t, orphanSummaries)

	var stats models.WorkspaceDashboardStats
	require.NoError(t, db.First(&stats, "workspace_id = ?", workspaceID).Error)
	require.Equal(t, int64(1), stats.TotalProjects)
	require.Equal(t, int64(1), stats.TotalPublishedPublications)
	require.Equal(t, int64(1), stats.TotalFailedPublications)
	require.Equal(t, int64(1), stats.TotalMembers)

	var orphanStats int64
	require.NoError(t, db.Model(&models.WorkspaceDashboardStats{}).Where("workspace_id = ?", orphanWorkspaceID).Count(&orphanStats).Error)
	require.Zero(t, orphanStats)
}

func TestRebuildDashboardBatchesAllProjectsWhenCreatedAtOrderDiffersFromPrimaryKey(t *testing.T) {
	db := testsupport.SetupTestDB()
	service := readmodel.NewService(db)

	userID := uuid.New()
	workspaceID := uuid.New()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.User{ID: userID, Username: "batch-owner", Email: "batch-owner@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.Workspace{ID: workspaceID, OwnerUserID: userID, Name: "Batch", Status: models.WorkspaceStatusActive, CreatedAt: now, UpdatedAt: now}).Error)

	const totalProjects = 201
	for i := range totalProjects {
		projectID := uuid.New()
		if i == 199 {
			projectID = uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
		}
		if i == 200 {
			projectID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
		}
		require.NoError(t, db.Create(&models.Project{
			ID:            projectID,
			UserID:        userID,
			WorkspaceID:   &workspaceID,
			Title:         "Batch project",
			SourceContent: "content",
			Status:        models.ProjectStatusReady,
			CreatedAt:     now.Add(time.Duration(totalProjects-i) * time.Second),
			UpdatedAt:     now,
		}).Error)
	}

	result, err := service.RebuildDashboard()
	require.NoError(t, err)
	require.Equal(t, int64(totalProjects), result.ProjectsRefreshed)

	var summaries int64
	require.NoError(t, db.Model(&models.ProjectListSummary{}).Count(&summaries).Error)
	require.Equal(t, int64(totalProjects), summaries)
}
