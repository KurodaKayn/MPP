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

type ExtensionSessionUser struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
}

type ExtensionSessionResponse struct {
	Authenticated bool                 `json:"authenticated"`
	User          ExtensionSessionUser `json:"user"`
}

type ExtensionPrepublishPlatform struct {
	PublicationID uuid.UUID `json:"publication_id"`
	Platform      string    `json:"platform"`
	AdapterKey    string    `json:"adapter_key"`
	ContentKind   string    `json:"content_kind"`
	Status        string    `json:"status"`
	Enabled       bool      `json:"enabled"`
	Preview       string    `json:"preview"`
}

type ExtensionPrepublishItem struct {
	ProjectID uuid.UUID                     `json:"project_id"`
	Title     string                        `json:"title"`
	Status    string                        `json:"status"`
	UpdatedAt time.Time                     `json:"updated_at"`
	Platforms []ExtensionPrepublishPlatform `json:"platforms"`
}

type ExtensionPrepublishResponse struct {
	Items []ExtensionPrepublishItem `json:"items"`
}

type CreateExtensionHandoffRequest struct {
	ProjectID uuid.UUID `json:"project_id"`
	Platforms []string  `json:"platforms"`
}

type ExtensionHandoffProject struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

type ExtensionHandoffCallback struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type ExtensionHandoffAsset struct {
	Type      string `json:"type"`
	SourceURL string `json:"source_url"`
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
}

type ExtensionHandoffPlatform struct {
	Platform       string                   `json:"platform"`
	AdapterKey     string                   `json:"adapter_key"`
	InjectURL      string                   `json:"inject_url"`
	ContentKind    string                   `json:"content_kind"`
	AutoPublish    bool                     `json:"auto_publish"`
	RequiresReview bool                     `json:"requires_review"`
	AdaptedContent map[string]interface{}   `json:"adapted_content"`
	Assets         []ExtensionHandoffAsset  `json:"assets"`
	Callback       ExtensionHandoffCallback `json:"callback"`
}

type ExtensionPublishHandoff struct {
	SchemaVersion int                        `json:"schema_version"`
	Type          string                     `json:"type"`
	ExecutionID   string                     `json:"execution_id"`
	ExpiresAt     time.Time                  `json:"expires_at"`
	Project       ExtensionHandoffProject    `json:"project"`
	Platforms     []ExtensionHandoffPlatform `json:"platforms"`
}

type ExtensionEventCallbackRequest struct {
	Token        string                 `json:"token"`
	EventID      string                 `json:"event_id"`
	Platform     string                 `json:"platform"`
	Status       string                 `json:"status"`
	Message      string                 `json:"message"`
	RemoteID     string                 `json:"remote_id"`
	PublishURL   string                 `json:"publish_url"`
	ErrorMessage string                 `json:"error_message"`
	Metadata     map[string]interface{} `json:"metadata"`
}

type ExtensionEventCallbackResponse struct {
	Accepted  bool `json:"accepted"`
	Duplicate bool `json:"duplicate"`
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

type AddProjectCollaboratorRequest struct {
	UserID uuid.UUID `json:"user_id,omitempty"`
	Email  string    `json:"email,omitempty"`
	Role   string    `json:"role"`
}

type UpdateProjectCollaboratorRequest struct {
	Role string `json:"role"`
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
	ID               uuid.UUID            `json:"id"`
	UserID           uuid.UUID            `json:"user_id"`
	CollabDocumentID *uuid.UUID           `json:"collab_document_id,omitempty"`
	Title            string               `json:"title"`
	Status           string               `json:"status"`
	Role             string               `json:"role"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	Publications     []PublicationSummary `json:"publications"`
}

type ProjectDetail struct {
	ID               uuid.UUID            `json:"id"`
	UserID           uuid.UUID            `json:"user_id"`
	CollabDocumentID *uuid.UUID           `json:"collab_document_id,omitempty"`
	Title            string               `json:"title"`
	SourceContent    string               `json:"source_content"`
	Status           string               `json:"status"`
	Role             string               `json:"role"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	Publications     []PublicationSummary `json:"publications"`
}

type ProjectCollaborator struct {
	ProjectID uuid.UUID `json:"project_id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectCollaboratorsResponse struct {
	Items []ProjectCollaborator `json:"items"`
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
