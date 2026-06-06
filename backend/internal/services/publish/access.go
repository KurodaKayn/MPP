package publish

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *Service) projectForPublish(ctx context.Context, projectID uuid.UUID, userID uuid.UUID) (models.Project, error) {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return models.Project{}, ErrForbidden
	}

	db := s.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}

	var project models.Project
	if err := db.First(&project, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Project{}, ErrForbidden
		}
		return models.Project{}, err
	}
	if project.UserID != userID {
		if ok, err := canPublishProjectWithDB(db, project, userID); err != nil {
			return models.Project{}, err
		} else if !ok {
			return models.Project{}, ErrForbidden
		}
	}
	return project, nil
}

func canPublishProjectWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (bool, error) {
	var collaborator models.ProjectCollaborator
	if err := db.Select("role").
		First(&collaborator, "project_id = ? AND user_id = ?", project.ID, userID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	} else {
		return collaborator.Role == models.ProjectRoleOwner || collaborator.Role == models.ProjectRoleEditor, nil
	}

	if project.WorkspaceID == nil || *project.WorkspaceID == uuid.Nil {
		return false, nil
	}
	var workspace models.Workspace
	if err := db.Select("owner_user_id").First(&workspace, "id = ?", *project.WorkspaceID).Error; err != nil {
		return false, err
	}
	if workspace.OwnerUserID == userID {
		return true, nil
	}

	var member models.WorkspaceMember
	if err := db.Select("role").
		First(&member, "workspace_id = ? AND user_id = ?", *project.WorkspaceID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return member.Role == models.WorkspaceRoleAdmin || member.Role == models.WorkspaceRoleMember, nil
}
