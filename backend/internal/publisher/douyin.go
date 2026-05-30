package publisher

import (
	"context"
	"fmt"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

type DouyinPublisher struct{}

func (d *DouyinPublisher) ValidateConfig(config []byte) error {
	// Douyin specific configuration validation (e.g. hashtags, publish time)
	return nil
}

func (d *DouyinPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	// Douyin usually takes plain text for description
	return []byte(project.SourceContent), nil
}

func (d *DouyinPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("douyin headless publishing requires an account with cookies")
	}

	// Implementation will follow in subsequent steps
	return "douyin_placeholder_id", "https://creator.douyin.com/content/manage", nil
}
