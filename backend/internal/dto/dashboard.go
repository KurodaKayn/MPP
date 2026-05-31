package dto

import (
	"time"

	"github.com/google/uuid"
)

type PaginationResponse struct {
	Items      interface{} `json:"items"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	Total      int64       `json:"total"`
	TotalPages int         `json:"total_pages"`
}

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type DashboardStatsResponse struct {
	TotalUsers                 int64 `json:"total_users"`
	TotalProjects              int64 `json:"total_projects"`
	TotalPublishedPublications int64 `json:"total_published_publications"`
	TotalFailedPublications    int64 `json:"total_failed_publications"`
}

type CreateProjectRequest struct {
	Title         string   `json:"title"`
	SourceContent string   `json:"source_content"`
	Summary       string   `json:"summary,omitempty"`
	CoverImageURL string   `json:"cover_image_url,omitempty"`
	Platforms     []string `json:"platforms"`
}

type UpdateProjectRequest struct {
	Title         string   `json:"title"`
	SourceContent string   `json:"source_content"`
	Summary       string   `json:"summary,omitempty"`
	CoverImageURL string   `json:"cover_image_url,omitempty"`
	Platforms     []string `json:"platforms"`
}

type SaveProjectContentRequest struct {
	Title         string `json:"title"`
	SourceContent string `json:"source_content"`
	Summary       string `json:"summary,omitempty"`
	CoverImageURL string `json:"cover_image_url,omitempty"`
}

type SaveProjectPlatformsRequest struct {
	Platforms []string `json:"platforms"`
}

type SyncActor struct {
	Type string `json:"type"`
}

type SyncPrepublishRequest struct {
	Platforms []string  `json:"platforms"`
	Actor     SyncActor `json:"actor"`
}

type UpdatePrepublishDraftRequest struct {
	AdaptedContent map[string]interface{} `json:"adapted_content"`
}

type AIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AIEditContentRequest struct {
	Title        string          `json:"title,omitempty"`
	Content      string          `json:"content"`
	Message      string          `json:"message"`
	Conversation []AIChatMessage `json:"conversation,omitempty"`
}

type AIEditContentResponse struct {
	Channel string `json:"channel"`
	Content string `json:"content"`
}

type AIEditPrepublishRequest struct {
	Title          string                 `json:"title,omitempty"`
	Platform       string                 `json:"platform"`
	AdaptedContent map[string]interface{} `json:"adapted_content"`
	Message        string                 `json:"message"`
	Conversation   []AIChatMessage        `json:"conversation,omitempty"`
}

type AIEditPrepublishResponse struct {
	Channel        string                 `json:"channel"`
	Platform       string                 `json:"platform"`
	AdaptedContent map[string]interface{} `json:"adapted_content"`
	Content        string                 `json:"content"`
}

type PublicationSummary struct {
	ID         uuid.UUID `json:"id"`
	Platform   string    `json:"platform"`
	Enabled    bool      `json:"enabled"`
	Status     string    `json:"status"`
	PublishURL string    `json:"publish_url,omitempty"`
}

type ProjectListItem struct {
	ID           uuid.UUID            `json:"id"`
	UserID       uuid.UUID            `json:"user_id"`
	Title        string               `json:"title"`
	Status       string               `json:"status"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Publications []PublicationSummary `json:"publications"`
}

type ProjectDetail struct {
	ID            uuid.UUID            `json:"id"`
	UserID        uuid.UUID            `json:"user_id"`
	Title         string               `json:"title"`
	SourceContent string               `json:"source_content"`
	Status        string               `json:"status"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	Publications  []PublicationSummary `json:"publications"`
}

type PublicationDetail struct {
	ID             uuid.UUID              `json:"id"`
	Platform       string                 `json:"platform"`
	Enabled        bool                   `json:"enabled"`
	Status         string                 `json:"status"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Config         map[string]interface{} `json:"config"`
	AdaptedContent map[string]interface{} `json:"adapted_content"`
	PublishURL     string                 `json:"publish_url,omitempty"`
	RemoteID       string                 `json:"remote_id,omitempty"`
	RetryCount     int                    `json:"retry_count"`
	LastAttemptAt  *time.Time             `json:"last_attempt_at,omitempty"`
	PublishedAt    *time.Time             `json:"published_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type ProjectPublicationsResponse struct {
	ProjectID uuid.UUID           `json:"project_id"`
	Items     []PublicationDetail `json:"items"`
}
