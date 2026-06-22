package project

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *Service) DeleteProject(projectID uuid.UUID, userID uuid.UUID) error {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return ErrInvalidProject
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id", "status").
		First(&project, "id = ?", projectID).Error; err != nil {
		return err
	}

	if err := s.authorizeProjectDelete(project, userID); err != nil {
		return err
	}
	if err := s.ensureProjectDeleteNotBlocked(projectID); err != nil {
		return err
	}

	workspaceID := projectWorkspaceID(project)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var scheduleIDs []uuid.UUID
		if tx.Migrator().HasTable(&models.ScheduledPublication{}) {
			if err := tx.Model(&models.ScheduledPublication{}).
				Where("project_id = ?", projectID).
				Pluck("id", &scheduleIDs).Error; err != nil {
				return err
			}
			if len(scheduleIDs) > 0 && tx.Migrator().HasTable(&models.PublishAttempt{}) {
				if err := tx.Where("scheduled_publication_id IN ?", scheduleIDs).
					Delete(&models.PublishAttempt{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("project_id = ?", projectID).
				Delete(&models.ScheduledPublication{}).Error; err != nil {
				return err
			}
		}

		cleanup := []any{
			&models.PublishEvent{},
			&models.ExtensionCallbackToken{},
			&models.MediaAssetUsage{},
			&models.PlatformAccountGrant{},
			&models.ProjectListSummary{},
			&models.ProjectPlatformPublication{},
			&models.ProjectCollaborator{},
			&models.ProjectActivity{},
			&models.ProjectComment{},
			&models.ProjectVersion{},
			&models.ProjectShareLink{},
		}
		for _, model := range cleanup {
			if !tx.Migrator().HasTable(model) {
				continue
			}
			if err := tx.Where("project_id = ?", projectID).Delete(model).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable(&models.ExtensionExecutionEvent{}) {
			var extensionEventIDs []uuid.UUID
			if err := tx.Model(&models.ExtensionExecutionEvent{}).
				Where("project_id = ?", projectID).
				Pluck("id", &extensionEventIDs).Error; err != nil {
				return err
			}
			if len(extensionEventIDs) > 0 && tx.Migrator().HasTable(&models.ExtensionExecutionEventClaim{}) {
				if err := tx.Where("record_id IN ?", extensionEventIDs).
					Delete(&models.ExtensionExecutionEventClaim{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("project_id = ?", projectID).
				Delete(&models.ExtensionExecutionEvent{}).Error; err != nil {
				return err
			}
		}

		if tx.Migrator().HasTable(&models.MediaAsset{}) {
			if err := tx.Model(&models.MediaAsset{}).
				Where("project_id = ?", projectID).
				Update("project_id", nil).Error; err != nil {
				return err
			}
		}

		result := tx.Delete(&models.Project{}, "id = ?", projectID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		return err
	}

	s.invalidateDashboardCaches(true)
	s.refreshWorkspaceReadModel(workspaceID)
	return nil
}

func (s *Service) authorizeProjectDelete(project models.Project, userID uuid.UUID) error {
	if project.UserID == userID {
		return nil
	}
	if project.WorkspaceID == nil || *project.WorkspaceID == uuid.Nil {
		return ErrForbidden
	}

	var workspace models.Workspace
	if err := s.db.Select("id", "owner_user_id").
		First(&workspace, "id = ?", *project.WorkspaceID).Error; err != nil {
		return err
	}
	if workspace.OwnerUserID == userID {
		return nil
	}

	var member models.WorkspaceMember
	if err := s.db.Select("workspace_id", "user_id", "role").
		First(&member, "workspace_id = ? AND user_id = ?", *project.WorkspaceID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrForbidden
		}
		return err
	}
	if member.Role != models.WorkspaceRoleAdmin {
		return ErrForbidden
	}
	return nil
}

func (s *Service) ensureProjectDeleteNotBlocked(projectID uuid.UUID) error {
	var activePublications int64
	if err := s.db.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND status IN ?", projectID, []string{
			models.PublicationStatusQueued,
			models.PublicationStatusPublishing,
		}).
		Count(&activePublications).Error; err != nil {
		return err
	}
	if activePublications > 0 {
		return ErrProjectDeletionBlocked
	}

	if !s.db.Migrator().HasTable(&models.ScheduledPublication{}) {
		return nil
	}

	var activeSchedules int64
	if err := s.db.Model(&models.ScheduledPublication{}).
		Where("project_id = ? AND status IN ?", projectID, []string{
			models.ScheduledPublicationStatusRunning,
			models.ScheduledPublicationStatusNeedsManualAction,
		}).
		Count(&activeSchedules).Error; err != nil {
		return err
	}
	if activeSchedules > 0 {
		return ErrProjectDeletionBlocked
	}
	return nil
}

func (s *Service) refreshWorkspaceReadModel(workspaceID uuid.UUID) {
	if s.readModels == nil || workspaceID == uuid.Nil {
		return
	}
	s.readModels.RefreshWorkspaceAsync(s.requestContext(), workspaceID)
}
