package core

import (
	"context"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

// PlatformPublisher defines the interface for all platform-specific publishing logic
type PlatformPublisher interface {
	ValidateConfig(config []byte) error
	AdaptContent(project *models.Project) ([]byte, error)
	Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error)
}

type AdaptedContent = contracts.AdaptedContent
type GeneratedBy = contracts.GeneratedBy
type AdaptedAsset = contracts.AdaptedAsset

func SystemAdaptedContent(project *models.Project, format, adapterID, summary string) AdaptedContent {
	return AdaptedContent{
		SchemaVersion: Int(1),
		Format:        contracts.DraftFormat(format),
		Summary:       String(summary),
		SourceRevision: String(
			project.UpdatedAt.UTC().Format(time.RFC3339Nano),
		),
		GeneratedBy: &GeneratedBy{
			Type:    contracts.GeneratedByTypeSystem,
			Id:      adapterID,
			Version: String("1"),
		},
	}
}

func String(value string) *string {
	return &value
}

func Int(value int) *int {
	return &value
}
