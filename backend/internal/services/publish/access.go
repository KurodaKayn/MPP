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
		return models.Project{}, ErrForbidden
	}
	return project, nil
}
