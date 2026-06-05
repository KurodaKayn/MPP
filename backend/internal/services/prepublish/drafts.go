package prepublish

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

func (s *Service) SyncProjectPrepublish(projectID uuid.UUID, userID uuid.UUID, req dto.SyncPrepublishRequest) (*dto.ProjectPublicationsResponse, error) {
	if err := s.projects.SyncProjectCollabSourceContent(projectID, userID); err != nil {
		return nil, err
	}

	var project models.Project
	if err := s.db.Preload("Publications").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.projects.ProjectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !projectsvc.CanEditProjectRole(role) {
		return nil, ErrForbidden
	}

	platforms, err := projectsvc.NormalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if len(platforms) == 0 {
		for _, publication := range project.Publications {
			if publication.Enabled && publication.Status != models.PublicationStatusDisabled {
				platforms = append(platforms, publication.Platform)
			}
		}
	}
	if len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	publications, err := s.ensurePrepublishPublications(&project, platforms)
	if err != nil {
		return nil, err
	}
	if prepublishHasActivePublish(publications) {
		return nil, publishsvc.ErrPublicationAlreadyPublishing
	}
	if err := s.markPrepublishSyncing(project.ID, platforms); err != nil {
		return nil, err
	}

	draftCompiler := s.draftCompiler
	if draftCompiler == nil {
		draftCompiler = defaultDraftCompiler()
	}
	compiledDrafts, err := draftCompiler.CompileProjectDrafts(s.requestContext(), &project, publications, platforms)
	if err != nil {
		if markErr := s.markPrepublishCompileFailure(project.ID, platforms, err); markErr != nil {
			return nil, markErr
		}
		return s.projects.GetProjectPublications(projectID, &userID, true)
	}

	if err := s.applyCompiledPrepublishDrafts(project.ID, platforms, compiledDrafts); err != nil {
		return nil, err
	}

	return s.projects.GetProjectPublications(projectID, &userID, true)
}

func prepublishHasActivePublish(publications []models.ProjectPlatformPublication) bool {
	for _, publication := range publications {
		if prepublishPublishStatusActive(publication.Status) {
			return true
		}
	}
	return false
}

func prepublishPublishStatusActive(status string) bool {
	return status == models.PublicationStatusQueued || status == models.PublicationStatusPublishing
}

func (s *Service) markPrepublishSyncing(projectID uuid.UUID, platforms []string) error {
	if len(platforms) == 0 {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var activeCount int64
		if err := tx.Model(&models.ProjectPlatformPublication{}).
			Where("project_id = ? AND platform IN ? AND status IN ?", projectID, platforms, []string{
				models.PublicationStatusQueued,
				models.PublicationStatusPublishing,
			}).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount > 0 {
			return publishsvc.ErrPublicationAlreadyPublishing
		}

		if err := tx.Model(&models.ProjectPlatformPublication{}).
			Where("project_id = ? AND platform IN ? AND status NOT IN ?", projectID, platforms, []string{
				models.PublicationStatusQueued,
				models.PublicationStatusPublishing,
			}).
			Updates(map[string]any{
				"error_message": "",
				"status":        models.PublicationStatusSyncing,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.ProjectPlatformPublication{}).
			Where("project_id = ? AND platform IN ? AND status IN ?", projectID, platforms, []string{
				models.PublicationStatusQueued,
				models.PublicationStatusPublishing,
			}).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount > 0 {
			return publishsvc.ErrPublicationAlreadyPublishing
		}

		return nil
	})
}

func (s *Service) ensurePrepublishPublications(project *models.Project, platforms []string) ([]models.ProjectPlatformPublication, error) {
	publications := make([]models.ProjectPlatformPublication, 0, len(platforms))
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			var publication models.ProjectPlatformPublication
			err := tx.Where("project_id = ? AND platform = ?", project.ID, platform).First(&publication).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				config, adaptedContent, status, err := projectsvc.BuildPendingPublicationPayload(project.Title, "", "")
				if err != nil {
					return err
				}
				publication = models.ProjectPlatformPublication{
					ProjectID:      project.ID,
					Platform:       platform,
					Enabled:        true,
					Status:         status,
					Config:         config,
					AdaptedContent: adaptedContent,
				}
				if err := tx.Create(&publication).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				if err := tx.Model(&publication).Updates(map[string]any{
					"enabled": true,
					"status":  models.PublicationStatusDraft,
				}).Error; err != nil {
					return err
				}
				publication.Enabled = true
				publication.Status = models.PublicationStatusDraft
			}

			publications = append(publications, publication)
		}

		return nil
	}); err != nil {
		return nil, err
	}
	return publications, nil
}

func (s *Service) applyCompiledPrepublishDrafts(projectID uuid.UUID, platforms []string, drafts map[string][]byte) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			adaptedContent, ok := drafts[platform]
			if !ok {
				if err := updateSyncingPrepublishPublication(tx, projectID, platform, map[string]any{
					"error_message": "content pipeline did not return a compiled draft",
					"status":        models.PublicationStatusFailed,
				}); err != nil {
					return err
				}
				continue
			}

			if err := updateSyncingPrepublishPublication(tx, projectID, platform, map[string]any{
				"adapted_content": datatypes.JSON(adaptedContent),
				"enabled":         true,
				"error_message":   "",
				"last_attempt_at": nil,
				"published_at":    nil,
				"publish_url":     "",
				"remote_id":       "",
				"retry_count":     0,
				"status":          models.PublicationStatusDraft,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) markPrepublishCompileFailure(projectID uuid.UUID, platforms []string, err error) error {
	if len(platforms) == 0 {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			if err := updateSyncingPrepublishPublication(tx, projectID, platform, map[string]any{
				"error_message": publishsvc.SanitizeUserFacingErrorMessage(err.Error()),
				"status":        models.PublicationStatusFailed,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func updateSyncingPrepublishPublication(tx *gorm.DB, projectID uuid.UUID, platform string, updates map[string]any) error {
	result := tx.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND platform = ? AND status = ?", projectID, platform, models.PublicationStatusSyncing).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return publishsvc.ErrPublicationAlreadyPublishing
	}
	return nil
}

func (s *Service) UpdateProjectPrepublishDraft(projectID uuid.UUID, userID uuid.UUID, platform string, req dto.UpdatePrepublishDraftRequest) (*dto.ProjectPublicationsResponse, error) {
	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.projects.ProjectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !projectsvc.CanEditProjectRole(role) {
		return nil, ErrForbidden
	}

	platforms, err := projectsvc.NormalizeProjectPlatforms([]string{platform})
	if err != nil || len(platforms) != 1 {
		return nil, ErrInvalidProject
	}
	if len(req.AdaptedContent) == 0 {
		return nil, ErrInvalidProject
	}

	adaptedContent, err := json.Marshal(req.AdaptedContent)
	if err != nil {
		return nil, err
	}

	var publication models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platforms[0]).First(&publication).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&publication).Updates(map[string]any{
		"adapted_content": datatypes.JSON(adaptedContent),
		"enabled":         true,
		"error_message":   "",
		"last_attempt_at": nil,
		"published_at":    nil,
		"publish_url":     "",
		"remote_id":       "",
		"retry_count":     0,
		"status":          models.PublicationStatusAdapted,
	}).Error; err != nil {
		return nil, err
	}

	return s.projects.GetProjectPublications(projectID, &userID, true)
}
