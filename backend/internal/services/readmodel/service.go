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

const asyncRefreshTimeout = 15 * time.Second

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

	var project models.Project
	err := s.db.Preload("Publications", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, project_id, platform, enabled, status, draft_status, review_status, sync_required, publish_url").
			Order("platform asc")
	}).First(&project, "id = ?", projectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Delete(&models.ProjectListSummary{}, "project_id = ?", projectID).Error
	}
	if err != nil {
		return err
	}

	summary, err := projectListSummary(project)
	if err != nil {
		return err
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id",
			"workspace_id",
			"title",
			"status",
			"publications",
			"created_at",
			"updated_at",
			"refreshed_at",
		}),
	}).Create(&summary).Error; err != nil {
		return err
	}

	return s.RefreshWorkspace(summary.WorkspaceID)
}

func (s *Service) RefreshWorkspace(workspaceID uuid.UUID) error {
	if s == nil || workspaceID == uuid.Nil {
		return nil
	}

	stats := models.WorkspaceDashboardStats{
		WorkspaceID: workspaceID,
		RefreshedAt: time.Now().UTC(),
	}
	if err := s.db.Model(&models.Project{}).
		Where("workspace_id = ?", workspaceID).
		Count(&stats.TotalProjects).Error; err != nil {
		return err
	}
	if err := s.countWorkspacePublications(workspaceID, models.PublicationStatusPublished, &stats.TotalPublishedPublications); err != nil {
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
	return s.db.Model(&models.ProjectPlatformPublication{}).
		Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
		Where("projects.workspace_id = ?", workspaceID).
		Where("project_platform_publications.status = ?", status).
		Count(count).Error
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
		ProjectID:    project.ID,
		UserID:       project.UserID,
		WorkspaceID:  *workspaceID,
		Title:        project.Title,
		Status:       project.Status,
		Publications: datatypes.JSON(payload),
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    project.UpdatedAt,
		RefreshedAt:  time.Now().UTC(),
	}, nil
}
