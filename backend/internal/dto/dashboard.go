package dto

import (
	"time"

	"github.com/google/uuid"
)

type PaginationResponse struct {
	Items      any   `json:"items"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
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
	AdaptedContent map[string]any           `json:"adapted_content"`
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
	Token        string         `json:"token"`
	EventID      string         `json:"event_id"`
	Platform     string         `json:"platform"`
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	RemoteID     string         `json:"remote_id"`
	PublishURL   string         `json:"publish_url"`
	ErrorMessage string         `json:"error_message"`
	Metadata     map[string]any `json:"metadata"`
}

type ExtensionEventCallbackResponse struct {
	Accepted  bool `json:"accepted"`
	Duplicate bool `json:"duplicate"`
}

type CreateProjectRequest struct {
	Title          string     `json:"title"`
	SourceContent  string     `json:"source_content"`
	Summary        string     `json:"summary,omitempty"`
	CoverImageURL  string     `json:"cover_image_url,omitempty"`
	Platforms      []string   `json:"platforms"`
	TemplateID     *uuid.UUID `json:"template_id,omitempty"`
	BrandProfileID *uuid.UUID `json:"brand_profile_id,omitempty"`
}

type UpdateProjectRequest struct {
	Title          string     `json:"title"`
	SourceContent  string     `json:"source_content"`
	Summary        string     `json:"summary,omitempty"`
	CoverImageURL  string     `json:"cover_image_url,omitempty"`
	Platforms      []string   `json:"platforms"`
	TemplateID     *uuid.UUID `json:"template_id,omitempty"`
	BrandProfileID *uuid.UUID `json:"brand_profile_id,omitempty"`
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

type CreateMediaUploadRequest struct {
	Filename     string   `json:"filename"`
	MimeType     string   `json:"mime_type"`
	SizeBytes    int64    `json:"size_bytes"`
	Usage        string   `json:"usage"`
	LibraryScope string   `json:"library_scope,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	AltText      string   `json:"alt_text,omitempty"`
	Source       string   `json:"source,omitempty"`
}

type CreateMediaUploadResponse struct {
	AssetID   uuid.UUID         `json:"asset_id"`
	ObjectRef string            `json:"object_ref"`
	UploadURL string            `json:"upload_url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type CompleteMediaUploadResponse struct {
	AssetID   uuid.UUID `json:"asset_id"`
	ObjectRef string    `json:"object_ref"`
	Status    string    `json:"status"`
}

type ResolveMediaAssetsRequest struct {
	AssetIDs []uuid.UUID `json:"asset_ids"`
}

type ResolvedMediaAsset struct {
	AssetID   uuid.UUID `json:"asset_id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ResolveMediaAssetsResponse struct {
	Items []ResolvedMediaAsset `json:"items"`
}

type ResolveMediaObjectRefRequest struct {
	ObjectRef string `json:"object_ref"`
}

type ResolveMediaObjectRefResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ContentTemplate struct {
	ID               uuid.UUID      `json:"id"`
	WorkspaceID      *uuid.UUID     `json:"workspace_id,omitempty"`
	OwnerUserID      *uuid.UUID     `json:"owner_user_id,omitempty"`
	Scope            string         `json:"scope"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	TitleTemplate    string         `json:"title_template"`
	SourceTemplate   string         `json:"source_template"`
	DefaultPlatforms []string       `json:"default_platforms"`
	PlatformConfig   map[string]any `json:"platform_config"`
	Tags             []string       `json:"tags"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type ContentTemplatesResponse struct {
	Items []ContentTemplate `json:"items"`
}

type CreateContentTemplateRequest struct {
	Scope            string         `json:"scope,omitempty"`
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	TitleTemplate    string         `json:"title_template"`
	SourceTemplate   string         `json:"source_template"`
	DefaultPlatforms []string       `json:"default_platforms"`
	PlatformConfig   map[string]any `json:"platform_config,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
}

type BrandProfile struct {
	ID           uuid.UUID `json:"id"`
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	CreatedBy    uuid.UUID `json:"created_by"`
	Name         string    `json:"name"`
	Voice        string    `json:"voice"`
	Audience     string    `json:"audience"`
	BannedWords  []string  `json:"banned_words"`
	CTA          string    `json:"cta"`
	LinkStrategy string    `json:"link_strategy"`
	DefaultTags  []string  `json:"default_tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BrandProfilesResponse struct {
	Items []BrandProfile `json:"items"`
}

type CreateBrandProfileRequest struct {
	Name         string   `json:"name"`
	Voice        string   `json:"voice,omitempty"`
	Audience     string   `json:"audience,omitempty"`
	BannedWords  []string `json:"banned_words,omitempty"`
	CTA          string   `json:"cta,omitempty"`
	LinkStrategy string   `json:"link_strategy,omitempty"`
	DefaultTags  []string `json:"default_tags,omitempty"`
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
	AdaptedContent map[string]any `json:"adapted_content"`
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
	Channel string   `json:"channel"`
	Content string   `json:"content"`
	Usage   *AIUsage `json:"usage,omitempty"`
}

type AIUsage struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	Cost         float64 `json:"cost"`
	Currency     string  `json:"currency"`
}

type AIEditPrepublishRequest struct {
	Title          string          `json:"title,omitempty"`
	Platform       string          `json:"platform"`
	AdaptedContent map[string]any  `json:"adapted_content"`
	Message        string          `json:"message"`
	Conversation   []AIChatMessage `json:"conversation,omitempty"`
}

type AIEditPrepublishResponse struct {
	Channel        string         `json:"channel"`
	Platform       string         `json:"platform"`
	AdaptedContent map[string]any `json:"adapted_content"`
	Content        string         `json:"content"`
	Usage          *AIUsage       `json:"usage,omitempty"`
}

type PublicationSummary struct {
	ID           uuid.UUID `json:"id"`
	Platform     string    `json:"platform"`
	Enabled      bool      `json:"enabled"`
	Status       string    `json:"status"`
	DraftStatus  string    `json:"draft_status"`
	ReviewStatus string    `json:"review_status"`
	SyncRequired bool      `json:"sync_required"`
	PublishURL   string    `json:"publish_url,omitempty"`
}

type ProjectListItem struct {
	ID               uuid.UUID            `json:"id"`
	UserID           uuid.UUID            `json:"user_id"`
	WorkspaceID      *uuid.UUID           `json:"workspace_id,omitempty"`
	CollabDocumentID *uuid.UUID           `json:"collab_document_id,omitempty"`
	TemplateID       *uuid.UUID           `json:"template_id,omitempty"`
	BrandProfileID   *uuid.UUID           `json:"brand_profile_id,omitempty"`
	Title            string               `json:"title"`
	Status           string               `json:"status"`
	Role             string               `json:"role"`
	AccessSource     string               `json:"access_source"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	Publications     []PublicationSummary `json:"publications"`
}

type ProjectDetail struct {
	ID                 uuid.UUID                 `json:"id"`
	UserID             uuid.UUID                 `json:"user_id"`
	WorkspaceID        *uuid.UUID                `json:"workspace_id,omitempty"`
	CollabDocumentID   *uuid.UUID                `json:"collab_document_id,omitempty"`
	TemplateID         *uuid.UUID                `json:"template_id,omitempty"`
	BrandProfileID     *uuid.UUID                `json:"brand_profile_id,omitempty"`
	Title              string                    `json:"title"`
	SourceContent      string                    `json:"source_content"`
	Status             string                    `json:"status"`
	Role               string                    `json:"role"`
	AccessSource       string                    `json:"access_source"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
	Publications       []PublicationSummary      `json:"publications"`
	PublicationDetails []PublicationDetail       `json:"publication_details,omitempty"`
	Comments           []ProjectComment          `json:"comments,omitempty"`
	Versions           []ProjectVersion          `json:"versions,omitempty"`
	Activities         []ProjectActivity         `json:"activities,omitempty"`
	Collaborators      []ProjectCollaborator     `json:"collaborators,omitempty"`
	ShareLinks         []ProjectShareLink        `json:"share_links,omitempty"`
	PermissionSources  []ProjectPermissionSource `json:"permission_sources,omitempty"`
}

type ProjectPermissionSource struct {
	Source string `json:"source"`
	Role   string `json:"role"`
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

type ProjectActivity struct {
	ID             uuid.UUID      `json:"id"`
	ProjectID      uuid.UUID      `json:"project_id"`
	ActorUserID    uuid.UUID      `json:"actor_user_id"`
	ActorUsername  string         `json:"actor_username"`
	ActorEmail     string         `json:"actor_email"`
	TargetUserID   *uuid.UUID     `json:"target_user_id,omitempty"`
	TargetUsername string         `json:"target_username,omitempty"`
	TargetEmail    string         `json:"target_email,omitempty"`
	EventType      string         `json:"event_type"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
}

type ProjectActivitiesResponse struct {
	Items []ProjectActivity `json:"items"`
}

type CreateProjectCommentRequest struct {
	Body       string         `json:"body"`
	AnchorText string         `json:"anchor_text,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type UpdateProjectCommentRequest struct {
	Status string `json:"status"`
}

type ProjectComment struct {
	ID             uuid.UUID      `json:"id"`
	ProjectID      uuid.UUID      `json:"project_id"`
	AuthorID       uuid.UUID      `json:"author_id"`
	AuthorUsername string         `json:"author_username"`
	AuthorEmail    string         `json:"author_email"`
	Body           string         `json:"body"`
	AnchorText     string         `json:"anchor_text,omitempty"`
	Status         string         `json:"status"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
	ResolvedAt     *time.Time     `json:"resolved_at,omitempty"`
}

type ProjectCommentsResponse struct {
	Items []ProjectComment `json:"items"`
}

type ProjectVersion struct {
	ID               uuid.UUID  `json:"id"`
	ProjectID        uuid.UUID  `json:"project_id"`
	CreatedBy        uuid.UUID  `json:"created_by"`
	CreatorUsername  string     `json:"creator_username"`
	CreatorEmail     string     `json:"creator_email"`
	VersionNumber    int        `json:"version_number"`
	Title            string     `json:"title"`
	Source           string     `json:"source"`
	CollabDocumentID *uuid.UUID `json:"collab_document_id,omitempty"`
	CollabSeq        int64      `json:"collab_seq"`
	CreatedAt        time.Time  `json:"created_at"`
}

type ProjectVersionsResponse struct {
	Items []ProjectVersion `json:"items"`
}

type RestoreProjectVersionResponse struct {
	Project *ProjectDetail `json:"project"`
	Version ProjectVersion `json:"version"`
}

type CreateProjectShareLinkRequest struct {
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type ProjectShareLink struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID uuid.UUID  `json:"project_id"`
	CreatedBy uuid.UUID  `json:"created_by"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type ProjectShareLinkWithToken struct {
	ProjectShareLink
	Token string `json:"token"`
	URL   string `json:"url"`
}

type ProjectShareLinksResponse struct {
	Items []ProjectShareLink `json:"items"`
}

type AcceptProjectShareLinkResponse struct {
	Project *ProjectDetail `json:"project"`
	Role    string         `json:"role"`
}

type CreateWorkspaceRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

type UpdateWorkspaceRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

type AddWorkspaceMemberRequest struct {
	UserID uuid.UUID `json:"user_id,omitempty"`
	Email  string    `json:"email,omitempty"`
	Role   string    `json:"role"`
}

type CreateWorkspaceInviteRequest struct {
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type AcceptWorkspaceInviteRequest struct {
	Token string `json:"token"`
}

type UpdateWorkspaceMemberRequest struct {
	Role string `json:"role"`
}

type Workspace struct {
	ID          uuid.UUID `json:"id"`
	OwnerUserID uuid.UUID `json:"owner_user_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug,omitempty"`
	Status      string    `json:"status"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkspacesResponse struct {
	Items []Workspace `json:"items"`
}

type WorkspaceMember struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	UserID      uuid.UUID  `json:"user_id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	InvitedBy   *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt    *time.Time `json:"joined_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type WorkspaceMembersResponse struct {
	Items []WorkspaceMember `json:"items"`
}

type WorkspaceInvite struct {
	ID          uuid.UUID  `json:"id"`
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	InvitedBy   uuid.UUID  `json:"invited_by"`
	AcceptedBy  *uuid.UUID `json:"accepted_by,omitempty"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type WorkspaceInviteWithToken struct {
	WorkspaceInvite
	Token string `json:"token"`
}

type WorkspaceInvitesResponse struct {
	Items []WorkspaceInvite `json:"items"`
}

type WorkspaceActivity struct {
	ID             uuid.UUID      `json:"id"`
	WorkspaceID    uuid.UUID      `json:"workspace_id"`
	ActorUserID    uuid.UUID      `json:"actor_user_id"`
	ActorUsername  string         `json:"actor_username"`
	ActorEmail     string         `json:"actor_email"`
	TargetUserID   *uuid.UUID     `json:"target_user_id,omitempty"`
	TargetUsername string         `json:"target_username,omitempty"`
	TargetEmail    string         `json:"target_email,omitempty"`
	EventType      string         `json:"event_type"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
}

type WorkspaceActivitiesResponse struct {
	Items []WorkspaceActivity `json:"items"`
}

type PublicationDetail struct {
	ID             uuid.UUID      `json:"id"`
	Platform       string         `json:"platform"`
	Enabled        bool           `json:"enabled"`
	Status         string         `json:"status"`
	DraftStatus    string         `json:"draft_status"`
	ReviewStatus   string         `json:"review_status"`
	SyncRequired   bool           `json:"sync_required"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Config         map[string]any `json:"config"`
	AdaptedContent map[string]any `json:"adapted_content"`
	PublishURL     string         `json:"publish_url,omitempty"`
	RemoteID       string         `json:"remote_id,omitempty"`
	RetryCount     int            `json:"retry_count"`
	LastAttemptAt  *time.Time     `json:"last_attempt_at,omitempty"`
	PublishedAt    *time.Time     `json:"published_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type ProjectPublicationsResponse struct {
	ProjectID uuid.UUID           `json:"project_id"`
	Items     []PublicationDetail `json:"items"`
}
