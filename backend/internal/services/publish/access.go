package publish

import (
	"context"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
)

func (s *Service) projectForPublish(ctx context.Context, projectID uuid.UUID, userID uuid.UUID) (models.Project, error) {
	return accesspolicy.ProjectForPublishWithDB(s.strongReadDB(ctx), projectID, userID)
}
