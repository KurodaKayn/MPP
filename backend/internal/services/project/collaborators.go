package project

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

func normalizeProjectCollaboratorRole(role string) (string, error) {
	role = strings.TrimSpace(role)
	switch role {
	case models.ProjectRoleEditor, models.ProjectRoleViewer:
		return role, nil
	default:
		return "", ErrInvalidProjectCollaborator
	}
}

func (s *Service) ListProjectCollaborators(projectID uuid.UUID, actorUserID uuid.UUID) (*dto.ProjectCollaboratorsResponse, error) {
	if _, err := s.requireProjectOwner(projectID, actorUserID); err != nil {
		return nil, err
	}

	var collaborators []models.ProjectCollaborator
	if err := s.db.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("project_id = ?", projectID).
		Order("created_at asc").
		Find(&collaborators).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectCollaborator, 0, len(collaborators))
	for _, collaborator := range collaborators {
		items = append(items, projectCollaboratorFromModel(collaborator))
	}
	return &dto.ProjectCollaboratorsResponse{Items: items}, nil
}

func (s *Service) ListOwnedProjectCollaboratorSummaries(actorUserID uuid.UUID) (*dto.ProjectCollaboratorSummariesResponse, error) {
	if actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}

	var collaborators []models.ProjectCollaborator
	if err := s.db.
		Joins("JOIN projects ON projects.id = project_collaborators.project_id").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("projects.user_id = ?", actorUserID).
		Order("project_collaborators.project_id asc").
		Order("project_collaborators.created_at asc").
		Find(&collaborators).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectCollaboratorSummary, 0)
	indexByProjectID := make(map[uuid.UUID]int)
	for _, collaborator := range collaborators {
		index, ok := indexByProjectID[collaborator.ProjectID]
		if !ok {
			index = len(items)
			indexByProjectID[collaborator.ProjectID] = index
			items = append(items, dto.ProjectCollaboratorSummary{
				ProjectID:     collaborator.ProjectID,
				Collaborators: make([]dto.ProjectCollaborator, 0, 1),
			})
		}

		items[index].Collaborators = append(items[index].Collaborators, projectCollaboratorFromModel(collaborator))
		items[index].CollaboratorCount = len(items[index].Collaborators)
	}

	return &dto.ProjectCollaboratorSummariesResponse{Items: items}, nil
}

func (s *Service) AddProjectCollaborator(projectID uuid.UUID, actorUserID uuid.UUID, req dto.AddProjectCollaboratorRequest) (*dto.ProjectCollaborator, error) {
	project, err := s.requireProjectOwner(projectID, actorUserID)
	if err != nil {
		return nil, err
	}

	role, err := normalizeProjectCollaboratorRole(req.Role)
	if err != nil {
		return nil, err
	}

	user, err := s.resolveProjectCollaboratorUser(req)
	if err != nil {
		return nil, err
	}
	if user.ID == project.UserID {
		return nil, ErrInvalidProjectCollaborator
	}

	collaborator := models.ProjectCollaborator{
		ProjectID: projectID,
		UserID:    user.ID,
		Role:      role,
		CreatedBy: actorUserID,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "project_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"role":       role,
				"created_by": actorUserID,
			}),
		}).Create(&collaborator).Error; err != nil {
			return err
		}
		return recordProjectActivity(tx, projectID, actorUserID, &user.ID, models.ProjectActivityCollaboratorAdded, map[string]any{
			"role": role,
		})
	}); err != nil {
		return nil, err
	}

	s.invalidateDashboardCaches(false)
	s.invalidateDashboardScopedStatsCache()

	return s.getProjectCollaborator(projectID, user.ID)
}

func (s *Service) UpdateProjectCollaborator(projectID uuid.UUID, actorUserID uuid.UUID, targetUserID uuid.UUID, req dto.UpdateProjectCollaboratorRequest) (*dto.ProjectCollaborator, error) {
	project, err := s.requireProjectOwner(projectID, actorUserID)
	if err != nil {
		return nil, err
	}
	if targetUserID == uuid.Nil || targetUserID == project.UserID {
		return nil, ErrInvalidProjectCollaborator
	}

	role, err := normalizeProjectCollaboratorRole(req.Role)
	if err != nil {
		return nil, err
	}

	var collaborator models.ProjectCollaborator
	if err := s.db.Where("project_id = ? AND user_id = ?", projectID, targetUserID).First(&collaborator).Error; err != nil {
		return nil, err
	}
	previousRole := collaborator.Role
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&collaborator).Update("role", role).Error; err != nil {
			return err
		}
		return recordProjectActivity(tx, projectID, actorUserID, &targetUserID, models.ProjectActivityCollaboratorRoleChanged, map[string]any{
			"previous_role": previousRole,
			"role":          role,
		})
	}); err != nil {
		return nil, err
	}

	s.invalidateDashboardCaches(false)

	return s.getProjectCollaborator(projectID, targetUserID)
}

func (s *Service) RemoveProjectCollaborator(projectID uuid.UUID, actorUserID uuid.UUID, targetUserID uuid.UUID) error {
	project, err := s.requireProjectOwner(projectID, actorUserID)
	if err != nil {
		return err
	}
	if targetUserID == uuid.Nil || targetUserID == project.UserID {
		return ErrInvalidProjectCollaborator
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&models.ProjectCollaborator{}, "project_id = ? AND user_id = ?", projectID, targetUserID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return recordProjectActivity(tx, projectID, actorUserID, &targetUserID, models.ProjectActivityCollaboratorRemoved, nil)
	}); err != nil {
		return err
	}

	s.invalidateDashboardCaches(false)
	s.invalidateDashboardScopedStatsCache()

	return nil
}

func (s *Service) resolveProjectCollaboratorUser(req dto.AddProjectCollaboratorRequest) (*models.User, error) {
	var user models.User
	if req.UserID != uuid.Nil {
		if err := s.db.Select("id", "username", "email").First(&user, "id = ?", req.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		return nil, ErrInvalidProjectCollaborator
	}
	if err := s.db.
		Select("id", "username", "email").
		Where("LOWER(email) = LOWER(?)", email).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) getProjectCollaborator(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectCollaborator, error) {
	var collaborator models.ProjectCollaborator
	if err := s.db.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		First(&collaborator).Error; err != nil {
		return nil, err
	}
	item := projectCollaboratorFromModel(collaborator)
	return &item, nil
}

func projectCollaboratorFromModel(collaborator models.ProjectCollaborator) dto.ProjectCollaborator {
	return dto.ProjectCollaborator{
		ProjectID: collaborator.ProjectID,
		UserID:    collaborator.UserID,
		Username:  collaborator.User.Username,
		Email:     collaborator.User.Email,
		Role:      collaborator.Role,
		CreatedBy: collaborator.CreatedBy,
		CreatedAt: collaborator.CreatedAt,
	}
}
