package publisher

import (
	"context"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

// PlatformPublisher defines the interface for all platform-specific publishing logic
type PlatformPublisher interface {
	ValidateConfig(config []byte) error
	AdaptContent(project *models.Project) ([]byte, error)
	Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error)
}

