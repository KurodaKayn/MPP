package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	asyncRefreshTimeout       = 15 * time.Second
	rebuildProjectBatchSize   = 200
	rebuildWorkspaceBatchSize = 200
)

type RebuildDashboardResult struct {
	ProjectsRefreshed             int64
	WorkspacesRefreshed           int64
	OrphanProjectSummariesDeleted int64
	OrphanWorkspaceStatsDeleted   int64
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	return &scoped
}

func (s *Service) RefreshProjectAsync(ctx context.Context, projectID uuid.UUID) {
	if s == nil || projectID == uuid.Nil {
		return
	}
	s.runAsync(ctx, func(ctx context.Context) error {
		return s.WithContext(ctx).RefreshProject(projectID)
	})
}

func (s *Service) RefreshWorkspaceAsync(ctx context.Context, workspaceID uuid.UUID) {
	if s == nil || workspaceID == uuid.Nil {
		return
	}
	s.runAsync(ctx, func(ctx context.Context) error {
		return s.WithContext(ctx).RefreshWorkspace(workspaceID)
	})
}

func (s *Service) runAsync(ctx context.Context, refresh func(context.Context) error) {
	parent := context.Background()
	if ctx != nil {
		parent = context.WithoutCancel(ctx)
	}
	if s.db != nil && s.db.Config != nil && s.db.Name() == "sqlite" {
		refreshCtx, cancel := context.WithTimeout(parent, asyncRefreshTimeout)
		defer cancel()
		if err := refresh(refreshCtx); err != nil {
			log.Printf("dashboard read model refresh failed: %v", err)
		}
		return
	}
	go func() {
		refreshCtx, cancel := context.WithTimeout(parent, asyncRefreshTimeout)
		defer cancel()
		if err := refresh(refreshCtx); err != nil {
			log.Printf("dashboard read model refresh failed: %v", err)
		}
	}()
}

func (s *Service) RefreshProject(projectID uuid.UUID) error {
	if s == nil || projectID == uuid.Nil {
		return nil
	}

	summary, err := s.refreshProjectSummary(projectID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Delete(&models.ProjectListSummary{}, "project_id = ?", projectID).Error
	}
	if err != nil {
		return err
	}

	return s.RefreshWorkspace(summary.WorkspaceID)
}

func (s *Service) refreshProjectSummary(projectID uuid.UUID) (models.ProjectListSummary, error) {
	var project models.Project
	err := s.db.Preload("Publications", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, project_id, platform, enabled, status, draft_status, review_status, sync_required, publish_url").
			Order("platform asc")
	}).First(&project, "id = ?", projectID).Error
	if err != nil {
		return models.ProjectListSummary{}, err
	}

	summary, err := projectListSummary(project)
	if err != nil {
		return models.ProjectListSummary{}, err
	}
	if err := s.upsertProjectSummaries([]models.ProjectListSummary{summary}); err != nil {
		return models.ProjectListSummary{}, err
	}

	return summary, nil
}

func (s *Service) upsertProjectSummaries(summaries []models.ProjectListSummary) error {
	if len(summaries) == 0 {
		return nil
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id",
			"workspace_id",
			"collab_document_id",
			"template_id",
			"brand_profile_id",
			"title",
			"status",
			"publications",
			"created_at",
			"updated_at",
			"refreshed_at",
		}),
	}).Create(&summaries).Error
}

func (s *Service) RefreshWorkspace(workspaceID uuid.UUID) error {
	if s == nil || workspaceID == uuid.Nil {
		return nil
	}

	stats := models.WorkspaceDashboardStats{
		WorkspaceID: workspaceID,
		RefreshedAt: time.Now().UTC(),
	}
	projectScope, err := s.workspaceProjectScope(workspaceID)
	if err != nil {
		return err
	}
	if err := projectScope.
		Count(&stats.TotalProjects).Error; err != nil {
		return err
	}
	if err := s.countWorkspacePublications(workspaceID, models.PublicationStatusSucceeded, &stats.TotalPublishedPublications); err != nil {
		return err
	}
	if err := s.countWorkspacePublications(workspaceID, models.PublicationStatusFailed, &stats.TotalFailedPublications); err != nil {
		return err
	}
	if err := s.db.Model(&models.WorkspaceMember{}).
		Where("workspace_id = ?", workspaceID).
		Count(&stats.TotalMembers).Error; err != nil {
		return err
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"total_projects",
			"total_published_publications",
			"total_failed_publications",
			"total_members",
			"refreshed_at",
		}),
	}).Create(&stats).Error
}

func (s *Service) countWorkspacePublications(workspaceID uuid.UUID, status string, count *int64) error {
	where, args, err := s.workspaceProjectWhere(workspaceID)
	if err != nil {
		return err
	}
	return s.db.Model(&models.ProjectPlatformPublication{}).
		Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
		Where(where, args...).
		Where("project_platform_publications.status = ?", status).
		Count(count).Error
}

func (s *Service) workspaceProjectScope(workspaceID uuid.UUID) (*gorm.DB, error) {
	where, args, err := s.workspaceProjectWhere(workspaceID)
	if err != nil {
		return nil, err
	}
	return s.db.Model(&models.Project{}).Where(where, args...), nil
}

func (s *Service) workspaceProjectWhere(workspaceID uuid.UUID) (string, []any, error) {
	var workspace models.Workspace
	err := s.db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "projects.workspace_id = ?", []any{workspaceID}, nil
	}
	if err != nil {
		return "", nil, err
	}
	if workspace.OwnerUserID != uuid.Nil && workspaceID == models.PersonalWorkspaceID(workspace.OwnerUserID) {
		return "(projects.workspace_id = ? OR (projects.workspace_id IS NULL AND projects.user_id = ?))", []any{workspaceID, workspace.OwnerUserID}, nil
	}
	return "projects.workspace_id = ?", []any{workspaceID}, nil
}

func (s *Service) RebuildDashboard() (RebuildDashboardResult, error) {
	var result RebuildDashboardResult
	if s == nil {
		return result, nil
	}

	deletedSummaries := s.db.Where(
		"project_id NOT IN (?)",
		s.db.Model(&models.Project{}).Select("id"),
	).Delete(&models.ProjectListSummary{})
	if deletedSummaries.Error != nil {
		return result, deletedSummaries.Error
	}
	result.OrphanProjectSummariesDeleted = deletedSummaries.RowsAffected

	var projects []models.Project
	if err := s.db.Omit("source_content").Preload("Publications", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, project_id, platform, enabled, status, draft_status, review_status, sync_required, publish_url").
			Order("platform asc")
	}).FindInBatches(&projects, rebuildProjectBatchSize, func(_ *gorm.DB, _ int) error {
		summaries := make([]models.ProjectListSummary, 0, len(projects))
		for _, project := range projects {
			summary, err := projectListSummary(project)
			if err != nil {
				return err
			}
			summaries = append(summaries, summary)
		}
		if err := s.upsertProjectSummaries(summaries); err != nil {
			return err
		}
		result.ProjectsRefreshed += int64(len(summaries))
		return nil
	}).Error; err != nil {
		return result, err
	}

	deletedStats := s.db.Where(
		"workspace_id NOT IN (?)",
		s.db.Model(&models.Workspace{}).Select("id"),
	).Delete(&models.WorkspaceDashboardStats{})
	if deletedStats.Error != nil {
		return result, deletedStats.Error
	}
	result.OrphanWorkspaceStatsDeleted = deletedStats.RowsAffected

	var workspaces []models.Workspace
	if err := s.db.Select("id").FindInBatches(&workspaces, rebuildWorkspaceBatchSize, func(_ *gorm.DB, _ int) error {
		for _, workspace := range workspaces {
			if err := s.RefreshWorkspace(workspace.ID); err != nil {
				return err
			}
			result.WorkspacesRefreshed++
		}
		return nil
	}).Error; err != nil {
		return result, err
	}

	return result, nil
}

func projectListSummary(project models.Project) (models.ProjectListSummary, error) {
	publications := make([]dto.PublicationSummary, 0, len(project.Publications))
	for _, pub := range project.Publications {
		publications = append(publications, dto.PublicationSummary{
			ID:           pub.ID,
			Platform:     pub.Platform,
			Enabled:      pub.Enabled,
			Status:       pub.Status,
			DraftStatus:  pub.DraftStatus,
			ReviewStatus: pub.ReviewStatus,
			SyncRequired: pub.SyncRequired,
			PublishURL:   pub.PublishURL,
		})
	}
	payload, err := json.Marshal(publications)
	if err != nil {
		return models.ProjectListSummary{}, err
	}
	workspaceID := project.WorkspaceID
	if workspaceID == nil || *workspaceID == uuid.Nil {
		personalWorkspaceID := models.PersonalWorkspaceID(project.UserID)
		workspaceID = &personalWorkspaceID
	}

	return models.ProjectListSummary{
		ProjectID:        project.ID,
		UserID:           project.UserID,
		WorkspaceID:      *workspaceID,
		CollabDocumentID: project.CollabDocumentID,
		TemplateID:       project.TemplateID,
		BrandProfileID:   project.BrandProfileID,
		Title:            project.Title,
		Status:           project.Status,
		Publications:     datatypes.JSON(payload),
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		RefreshedAt:      time.Now().UTC(),
	}, nil
}
