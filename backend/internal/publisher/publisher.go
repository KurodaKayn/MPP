package publisher

import (
	"context"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

// PlatformPublisher defines the interface for all platform-specific publishing logic
type PlatformPublisher interface {
	ValidateConfig(config []byte) error
	AdaptContent(project *models.Project) ([]byte, error)
	Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error)
}

type AdaptedContent struct {
	SchemaVersion  int            `json:"schema_version"`
	Format         string         `json:"format"`
	Summary        string         `json:"summary"`
	SourceRevision string         `json:"source_revision"`
	GeneratedBy    GeneratedBy    `json:"generated_by"`
	HTML           string         `json:"html,omitempty"`
	Markdown       string         `json:"markdown,omitempty"`
	Text           string         `json:"text,omitempty"`
	Assets         []AdaptedAsset `json:"assets,omitempty"`
}

type GeneratedBy struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	Version      string `json:"version,omitempty"`
	AgentRunID   string `json:"agent_run_id,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

type AdaptedAsset struct {
	Type      string `json:"type"`
	SourceURL string `json:"source_url"`
	Alt       string `json:"alt,omitempty"`
}

func systemAdaptedContent(project *models.Project, format, adapterID, summary string) AdaptedContent {
	return AdaptedContent{
		SchemaVersion:  1,
		Format:         format,
		Summary:        summary,
		SourceRevision: project.UpdatedAt.UTC().Format(time.RFC3339Nano),
		GeneratedBy: GeneratedBy{
			Type:    "system",
			ID:      adapterID,
			Version: "1",
		},
	}
}
