package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
)

// Project Status Constants
const (
	ProjectStatusDraft      = string(contracts.ProjectStatusDraft)
	ProjectStatusReady      = string(contracts.ProjectStatusReady)
	ProjectStatusPublishing = string(contracts.ProjectStatusPublishing)
	ProjectStatusPublished  = string(contracts.ProjectStatusPublished)
	ProjectStatusFailed     = string(contracts.ProjectStatusFailed)
)

// Publication Status Constants
const (
	PublicationStatusDraft      = string(contracts.PublicationStatusDraft)
	PublicationStatusSyncing    = string(contracts.PublicationStatusSyncing)
	PublicationStatusQueued     = string(contracts.PublicationStatusQueued)
	PublicationStatusPublishing = string(contracts.PublicationStatusPublishing)
	PublicationStatusSucceeded  = string(contracts.PublicationStatusSucceeded)
	PublicationStatusFailed     = string(contracts.PublicationStatusFailed)
	PublicationStatusCancelled  = string(contracts.PublicationStatusCancelled)

	// Deprecated compatibility aliases. New code should use draft/syncing/queued/
	// publishing/succeeded/failed/cancelled names.
	PublicationStatusPending   = PublicationStatusDraft
	PublicationStatusAdapted   = PublicationStatusDraft
	PublicationStatusPublished = PublicationStatusSucceeded
	PublicationStatusDisabled  = PublicationStatusCancelled
)

// Platform account status constants
const (
	PlatformAccountStatusUntested    = string(contracts.PlatformAccountStatusUntested)
	PlatformAccountStatusConnected   = string(contracts.PlatformAccountStatusConnected)
	PlatformAccountStatusFailed      = string(contracts.PlatformAccountStatusFailed)
	PlatformAccountStatusNeedsReauth = "needs_reauth"
)

const (
	PlatformAccountHealthUnknown     = "unknown"
	PlatformAccountHealthHealthy     = "healthy"
	PlatformAccountHealthNeedsReauth = "needs_reauth"
	PlatformAccountHealthFailed      = "failed"
)

const (
	PlatformAccountSharePrivate   = "private"
	PlatformAccountShareWorkspace = "workspace"
)

const (
	PlatformAccountGrantRoleManager   = "manager"
	PlatformAccountGrantRolePublisher = "publisher"
	PlatformAccountGrantRoleViewer    = "viewer"
)

// Remote Browser Session Status Constants
const (
	BrowserSessionStatusPending       = string(contracts.BrowserSessionStatusPending)
	BrowserSessionStatusReady         = string(contracts.BrowserSessionStatusReady)
	BrowserSessionStatusLoginDetected = string(contracts.BrowserSessionStatusLoginDetected)
	BrowserSessionStatusCapturing     = string(contracts.BrowserSessionStatusCapturing)
	BrowserSessionStatusConnected     = string(contracts.BrowserSessionStatusConnected)
	BrowserSessionStatusExpired       = string(contracts.BrowserSessionStatusExpired)
	BrowserSessionStatusFailed        = string(contracts.BrowserSessionStatusFailed)
)

type User struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Username              string    `gorm:"not null;uniqueIndex"`
	Email                 string    `gorm:"not null;uniqueIndex"`
	IsEmailVerified       bool      `gorm:"not null;default:false"`
	PasswordHash          string    `gorm:"not null"`
	Role                  string    `gorm:"not null;default:'user'"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Projects              []Project              `gorm:"foreignKey:UserID"`
	PlatformAccounts      []PlatformAccount      `gorm:"foreignKey:UserID"`
	RemoteBrowserSessions []RemoteBrowserSession `gorm:"foreignKey:UserID"`
}

type Project struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID           uuid.UUID  `gorm:"type:uuid;not null;index:idx_projects_user_status_created_at"`
	WorkspaceID      *uuid.UUID `gorm:"type:uuid;index:idx_projects_workspace_status_created_at"`
	CollabDocumentID *uuid.UUID `gorm:"type:uuid;uniqueIndex:ux_projects_collab_document"`
	TemplateID       *uuid.UUID `gorm:"type:uuid;index"`
	BrandProfileID   *uuid.UUID `gorm:"type:uuid;index"`
	Title            string     `gorm:"not null"`
	SourceContent    string     `gorm:"type:text;not null"`
	Status           string     `gorm:"not null;index:idx_projects_user_status_created_at;index:idx_projects_status_created_at;index:idx_projects_workspace_status_created_at"`
	CreatedAt        time.Time  `gorm:"index:idx_projects_user_status_created_at;index:idx_projects_status_created_at;index:idx_projects_workspace_status_created_at"`
	UpdatedAt        time.Time
	Publications     []ProjectPlatformPublication `gorm:"foreignKey:ProjectID"`
	Collaborators    []ProjectCollaborator        `gorm:"foreignKey:ProjectID"`
	CollabDocument   *CollabDocument              `gorm:"foreignKey:CollabDocumentID;references:ID;constraint:OnDelete:SET NULL"`
	Workspace        *Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:SET NULL"`
	Template         *ContentTemplate             `gorm:"foreignKey:TemplateID;references:ID;constraint:OnDelete:SET NULL"`
	BrandProfile     *BrandProfile                `gorm:"foreignKey:BrandProfileID;references:ID;constraint:OnDelete:SET NULL"`
}

const (
	MediaAssetStatusPending = "pending"
	MediaAssetStatusReady   = "ready"
	MediaAssetStatusFailed  = "failed"
	MediaAssetStatusDeleted = "deleted"

	MediaAssetUsageEditorImage = "editor_image"
	MediaAssetUsageCoverImage  = "cover_image"
)

const (
	ContentTemplateScopeSystem    = "system"
	ContentTemplateScopeWorkspace = "workspace"
	ContentTemplateScopePersonal  = "personal"
)

const (
	MediaAssetLibraryScopeProject   = "project"
	MediaAssetLibraryScopeWorkspace = "workspace"
	MediaAssetLibraryScopePersonal  = "personal"
)

const (
	PublicationDraftStatusUnsynced = "unsynced"
	PublicationDraftStatusSyncing  = "syncing"
	PublicationDraftStatusReady    = "ready"
	PublicationDraftStatusStale    = "stale"
)

const (
	PublicationReviewStatusDraft            = "draft"
	PublicationReviewStatusReviewing        = "reviewing"
	PublicationReviewStatusApproved         = "approved"
	PublicationReviewStatusChangesRequested = "changes_requested"
)

const (
	ScheduledPublicationStatusDraft             = "draft"
	ScheduledPublicationStatusPendingReview     = "pending_review"
	ScheduledPublicationStatusApproved          = "approved"
	ScheduledPublicationStatusScheduled         = "scheduled"
	ScheduledPublicationStatusRunning           = "running"
	ScheduledPublicationStatusPublished         = "published"
	ScheduledPublicationStatusFailed            = "failed"
	ScheduledPublicationStatusNeedsManualAction = "needs_manual_action"
	ScheduledPublicationStatusCancelled         = "cancelled"
)

const (
	PublishAttemptStatusRunning           = "running"
	PublishAttemptStatusSucceeded         = "succeeded"
	PublishAttemptStatusFailed            = "failed"
	PublishAttemptStatusNeedsManualAction = "needs_manual_action"
)

type ContentTemplate struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID      *uuid.UUID     `gorm:"type:uuid;index"`
	OwnerUserID      *uuid.UUID     `gorm:"type:uuid;index"`
	Scope            string         `gorm:"not null;index"`
	Name             string         `gorm:"not null"`
	Description      string         `gorm:"type:text;not null;default:''"`
	TitleTemplate    string         `gorm:"not null;default:''"`
	SourceTemplate   string         `gorm:"type:text;not null;default:''"`
	DefaultPlatforms datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	PlatformConfig   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Tags             datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	CreatedAt        time.Time      `gorm:"not null"`
	UpdatedAt        time.Time      `gorm:"not null"`
	Workspace        *Workspace     `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
	Owner            *User          `gorm:"foreignKey:OwnerUserID;constraint:OnDelete:CASCADE"`
}

type BrandProfile struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	CreatedBy    uuid.UUID      `gorm:"type:uuid;not null;index"`
	Name         string         `gorm:"not null"`
	Voice        string         `gorm:"type:text;not null;default:''"`
	Audience     string         `gorm:"type:text;not null;default:''"`
	BannedWords  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	CTA          string         `gorm:"type:text;not null;default:''"`
	LinkStrategy string         `gorm:"type:text;not null;default:''"`
	DefaultTags  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	CreatedAt    time.Time      `gorm:"not null"`
	UpdatedAt    time.Time      `gorm:"not null"`
	Workspace    Workspace      `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
	Creator      User           `gorm:"foreignKey:CreatedBy;constraint:OnDelete:CASCADE"`
}

type MediaAsset struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	UserID           uuid.UUID      `gorm:"type:uuid;not null;index"`
	WorkspaceID      *uuid.UUID     `gorm:"type:uuid;index"`
	ProjectID        *uuid.UUID     `gorm:"type:uuid;index"`
	DerivativeOf     *uuid.UUID     `gorm:"type:uuid;index"`
	Bucket           string         `gorm:"not null"`
	ObjectKey        string         `gorm:"not null;uniqueIndex"`
	OriginalFilename string         `gorm:"not null"`
	MimeType         string         `gorm:"not null;index"`
	SizeBytes        int64          `gorm:"not null"`
	Usage            string         `gorm:"not null;index"`
	LibraryScope     string         `gorm:"not null;default:'project';index"`
	Tags             datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	AltText          string         `gorm:"type:text;not null;default:''"`
	Source           string         `gorm:"not null;default:''"`
	Status           string         `gorm:"not null;index"`
	ETag             string         `gorm:"not null;default:''"`
	ErrorMessage     string         `gorm:"not null;default:''"`
	CreatedAt        time.Time      `gorm:"not null"`
	UpdatedAt        time.Time      `gorm:"not null"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	User             User           `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Workspace        *Workspace     `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:SET NULL"`
	Project          *Project       `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:SET NULL"`
}

type MediaAssetUsage struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	MediaAssetID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_media_asset_usages_asset_resource,priority:1"`
	WorkspaceID   uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProjectID     *uuid.UUID `gorm:"type:uuid;index"`
	PublicationID *uuid.UUID `gorm:"type:uuid;index"`
	TemplateID    *uuid.UUID `gorm:"type:uuid;index"`
	ResourceType  string     `gorm:"not null;uniqueIndex:idx_media_asset_usages_asset_resource,priority:2"`
	ResourceID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_media_asset_usages_asset_resource,priority:3"`
	UsageKind     string     `gorm:"not null;default:'';index"`
	CreatedAt     time.Time  `gorm:"not null"`
	Asset         MediaAsset `gorm:"foreignKey:MediaAssetID;constraint:OnDelete:CASCADE"`
	Workspace     Workspace  `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
}

const (
	ProjectRoleOwner  = "owner"
	ProjectRoleEditor = "editor"
	ProjectRoleViewer = "viewer"
)

const (
	ProjectActivityContentSaved            = string(contracts.ProjectActivityTypeContentSaved)
	ProjectActivityCommentCreated          = string(contracts.ProjectActivityTypeCommentCreated)
	ProjectActivityCommentResolved         = string(contracts.ProjectActivityTypeCommentResolved)
	ProjectActivityCollaboratorAdded       = string(contracts.ProjectActivityTypeCollaboratorAdded)
	ProjectActivityCollaboratorRoleChanged = string(contracts.ProjectActivityTypeCollaboratorRoleChanged)
	ProjectActivityCollaboratorRemoved     = string(contracts.ProjectActivityTypeCollaboratorRemoved)
	ProjectActivityPublishRequested        = string(contracts.ProjectActivityTypePublishRequested)
	ProjectActivityPublishQueued           = string(contracts.ProjectActivityTypePublishQueued)
	ProjectActivityPublishCompleted        = string(contracts.ProjectActivityTypePublishCompleted)
	ProjectActivityShareLinkAccepted       = string(contracts.ProjectActivityTypeShareLinkAccepted)
	ProjectActivityShareLinkCreated        = string(contracts.ProjectActivityTypeShareLinkCreated)
	ProjectActivityShareLinkRevoked        = string(contracts.ProjectActivityTypeShareLinkRevoked)
	ProjectActivityVersionRestored         = string(contracts.ProjectActivityTypeVersionRestored)
)

const (
	ProjectCommentStatusOpen     = string(contracts.ProjectCommentStatusOpen)
	ProjectCommentStatusResolved = string(contracts.ProjectCommentStatusResolved)
)

const (
	ProjectShareLinkStatusActive  = string(contracts.ProjectShareLinkStatusActive)
	ProjectShareLinkStatusRevoked = string(contracts.ProjectShareLinkStatusRevoked)
)

const (
	ProjectAccessSourceOwner       = "owner"
	ProjectAccessSourceDirectShare = "direct_share"
	ProjectAccessSourceWorkspace   = "workspace"
)

type ProjectCollaborator struct {
	ProjectID uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_project_collaborators_user_role"`
	Role      string    `gorm:"not null;index:idx_project_collaborators_user_role"`
	CreatedBy uuid.UUID `gorm:"type:uuid;not null;index"`
	CreatedAt time.Time
	Project   Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	User      User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Creator   User    `gorm:"foreignKey:CreatedBy"`
}

type ProjectActivity struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID    uuid.UUID      `gorm:"type:uuid;not null;index:idx_project_activities_project_created_at,priority:1"`
	ActorUserID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	TargetUserID *uuid.UUID     `gorm:"type:uuid;index"`
	EventType    string         `gorm:"not null;index"`
	Metadata     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt    time.Time      `gorm:"not null;index:idx_project_activities_project_created_at,priority:2"`
	Project      Project        `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Actor        User           `gorm:"foreignKey:ActorUserID;constraint:OnDelete:CASCADE"`
	TargetUser   *User          `gorm:"foreignKey:TargetUserID;constraint:OnDelete:SET NULL"`
}

type ProjectComment struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID  uuid.UUID      `gorm:"type:uuid;not null;index:idx_project_comments_project_created_at,priority:1"`
	AuthorID   uuid.UUID      `gorm:"type:uuid;not null;index"`
	Body       string         `gorm:"type:text;not null"`
	AnchorText string         `gorm:"type:text;not null;default:''"`
	Status     string         `gorm:"not null;default:'open';index"`
	Metadata   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time      `gorm:"not null;index:idx_project_comments_project_created_at,priority:2"`
	ResolvedAt *time.Time
	Project    Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Author     User    `gorm:"foreignKey:AuthorID;constraint:OnDelete:CASCADE"`
}

type ProjectVersion struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ProjectID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_project_versions_project_created_at,priority:1"`
	CreatedBy        uuid.UUID  `gorm:"type:uuid;not null;index"`
	VersionNumber    int        `gorm:"not null"`
	Title            string     `gorm:"not null"`
	SourceContent    string     `gorm:"type:text;not null"`
	CollabDocumentID *uuid.UUID `gorm:"type:uuid;index"`
	CollabSeq        int64      `gorm:"not null;default:0"`
	Source           string     `gorm:"not null"`
	CreatedAt        time.Time  `gorm:"not null;index:idx_project_versions_project_created_at,priority:2"`
	Project          Project    `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Creator          User       `gorm:"foreignKey:CreatedBy;constraint:OnDelete:CASCADE"`
}

type ProjectShareLink struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index"`
	CreatedBy uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"not null;uniqueIndex"`
	Role      string    `gorm:"not null"`
	Status    string    `gorm:"not null;default:'active';index"`
	ExpiresAt *time.Time
	CreatedAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
	Project   Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Creator   User    `gorm:"foreignKey:CreatedBy;constraint:OnDelete:CASCADE"`
}

const (
	WorkspaceStatusActive   = "active"
	WorkspaceStatusArchived = "archived"

	PersonalWorkspaceName = "Personal"

	WorkspaceRoleOwner  = "owner"
	WorkspaceRoleAdmin  = "admin"
	WorkspaceRoleMember = "member"
	WorkspaceRoleViewer = "viewer"
)

const (
	WorkspaceActivityWorkspaceCreated  = "workspace_created"
	WorkspaceActivityWorkspaceUpdated  = "workspace_updated"
	WorkspaceActivityMemberAdded       = "member_added"
	WorkspaceActivityMemberRoleChanged = "member_role_changed"
	WorkspaceActivityMemberRemoved     = "member_removed"
	WorkspaceActivityInviteCreated     = "invite_created"
	WorkspaceActivityInviteAccepted    = "invite_accepted"
	WorkspaceActivityInviteRevoked     = "invite_revoked"
)

var personalWorkspaceNamespace = uuid.MustParse("03d32585-3f8c-48a8-bf40-53aa3f1698c1")

func PersonalWorkspaceID(userID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(personalWorkspaceNamespace, []byte(userID.String()))
}

func PersonalWorkspaceSlug(userID uuid.UUID) string {
	return "personal-" + userID.String()
}

type Workspace struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	OwnerUserID uuid.UUID      `gorm:"type:uuid;not null;index"`
	Name        string         `gorm:"not null"`
	Slug        string         `gorm:"index"`
	Status      string         `gorm:"not null;default:'active';index"`
	CreatedAt   time.Time      `gorm:"not null"`
	UpdatedAt   time.Time      `gorm:"not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	Owner       User           `gorm:"foreignKey:OwnerUserID;constraint:OnDelete:CASCADE"`
	Members     []WorkspaceMember
	Projects    []Project
}

type WorkspaceMember struct {
	WorkspaceID uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID      uuid.UUID  `gorm:"type:uuid;primaryKey;index:idx_workspace_members_user_role"`
	Role        string     `gorm:"not null;index:idx_workspace_members_user_role"`
	InvitedBy   *uuid.UUID `gorm:"type:uuid;index"`
	JoinedAt    *time.Time
	CreatedAt   time.Time `gorm:"not null"`
	Workspace   Workspace `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
	User        User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Inviter     *User     `gorm:"foreignKey:InvitedBy"`
}

const (
	WorkspaceInviteStatusPending  = "pending"
	WorkspaceInviteStatusAccepted = "accepted"
	WorkspaceInviteStatusExpired  = "expired"
	WorkspaceInviteStatusRevoked  = "revoked"
)

type WorkspaceInvite struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_workspace_invites_workspace_status"`
	Email        string     `gorm:"not null;index:idx_workspace_invites_email_status"`
	Role         string     `gorm:"not null"`
	InvitedBy    uuid.UUID  `gorm:"type:uuid;not null;index"`
	AcceptedBy   *uuid.UUID `gorm:"type:uuid;index"`
	Status       string     `gorm:"not null;default:'pending';index:idx_workspace_invites_workspace_status;index:idx_workspace_invites_email_status"`
	TokenHash    string     `gorm:"not null;uniqueIndex"`
	ExpiresAt    time.Time  `gorm:"not null;index"`
	AcceptedAt   *time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
	Workspace    Workspace `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
	Inviter      User      `gorm:"foreignKey:InvitedBy;constraint:OnDelete:CASCADE"`
	AcceptedUser *User     `gorm:"foreignKey:AcceptedBy;constraint:OnDelete:SET NULL"`
}

type WorkspaceActivity struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID  uuid.UUID      `gorm:"type:uuid;not null;index:idx_workspace_activities_workspace_created_at,priority:1"`
	ActorUserID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	TargetUserID *uuid.UUID     `gorm:"type:uuid;index"`
	EventType    string         `gorm:"not null;index"`
	Metadata     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt    time.Time      `gorm:"not null;index:idx_workspace_activities_workspace_created_at,priority:2"`
	Workspace    Workspace      `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
	Actor        User           `gorm:"foreignKey:ActorUserID;constraint:OnDelete:CASCADE"`
	TargetUser   *User          `gorm:"foreignKey:TargetUserID;constraint:OnDelete:SET NULL"`
}

const (
	NotificationStatusUnread   = "unread"
	NotificationStatusRead     = "read"
	NotificationStatusArchived = "archived"

	NotificationAccountNeedsReauth = "account_needs_reauth"
)

type Notification struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID      `gorm:"type:uuid;not null;index"`
	RecipientUserID uuid.UUID      `gorm:"type:uuid;not null;index"`
	EventType       string         `gorm:"not null;index"`
	ResourceType    string         `gorm:"not null;default:'';index"`
	ResourceID      *uuid.UUID     `gorm:"type:uuid;index"`
	Status          string         `gorm:"not null;default:'unread';index"`
	Metadata        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt       time.Time      `gorm:"not null;index"`
	Workspace       Workspace      `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
	Recipient       User           `gorm:"foreignKey:RecipientUserID;constraint:OnDelete:CASCADE"`
}

type ProjectPlatformPublication struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_publications_project_platform"`
	Platform          string         `gorm:"not null;uniqueIndex:idx_publications_project_platform;index:idx_publications_platform_status"`
	PlatformAccountID *uuid.UUID     `gorm:"type:uuid;index"`
	Enabled           bool           `gorm:"not null;default:true"`
	Status            string         `gorm:"not null;index:idx_publications_platform_status"`
	DraftStatus       string         `gorm:"not null;default:'unsynced';index"`
	ReviewStatus      string         `gorm:"not null;default:'draft';index"`
	SyncRequired      bool           `gorm:"not null;default:false;index"`
	Config            datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	AdaptedContent    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	RemoteID          string
	PublishURL        string
	ErrorMessage      string
	RetryCount        int `gorm:"not null;default:0"`
	LastAttemptAt     *time.Time
	PublishedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PublishEvent struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	PublicationID  uuid.UUID `gorm:"type:uuid;not null;index"`
	ProjectID      uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index:idx_publish_events_user_idempotency"`
	Platform       string    `gorm:"not null;index"`
	JobID          uuid.UUID `gorm:"type:uuid;not null;index"`
	IdempotencyKey string    `gorm:"not null;index:idx_publish_events_user_idempotency"`
	EventType      string    `gorm:"not null;index"`
	Status         string    `gorm:"not null;index"`
	Message        string
	RemoteID       string
	PublishURL     string
	ErrorMessage   string
	Metadata       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt      time.Time
}

type ScheduledPublication struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_scheduled_publications_workspace_status_time,priority:1"`
	ProjectID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	PublicationID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	PlatformAccountID *uuid.UUID `gorm:"type:uuid;index"`
	ProjectVersionID  *uuid.UUID `gorm:"type:uuid;index"`
	ScheduledAt       time.Time  `gorm:"not null;index:idx_scheduled_publications_workspace_status_time,priority:3"`
	Timezone          string     `gorm:"not null;default:'UTC'"`
	Status            string     `gorm:"not null;index:idx_scheduled_publications_workspace_status_time,priority:2"`
	IdempotencyKey    string     `gorm:"not null;default:'';index"`
	CreatedBy         uuid.UUID  `gorm:"type:uuid;not null;index"`
	ApprovedBy        *uuid.UUID `gorm:"type:uuid;index"`
	CancelledBy       *uuid.UUID `gorm:"type:uuid;index"`
	LastError         string     `gorm:"type:text;not null;default:''"`
	ManualActionURL   string     `gorm:"type:text;not null;default:''"`
	ManualActionUntil *time.Time
	CreatedAt         time.Time                  `gorm:"not null"`
	UpdatedAt         time.Time                  `gorm:"not null"`
	Project           Project                    `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Publication       ProjectPlatformPublication `gorm:"foreignKey:PublicationID;constraint:OnDelete:CASCADE"`
	Workspace         Workspace                  `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
}

type PublishAttempt struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey"`
	ScheduledPublicationID uuid.UUID `gorm:"type:uuid;not null;index:idx_publish_attempts_schedule_attempt,priority:1"`
	AttemptNo              int       `gorm:"not null;index:idx_publish_attempts_schedule_attempt,priority:2"`
	StartedAt              time.Time `gorm:"not null"`
	FinishedAt             *time.Time
	Status                 string `gorm:"not null;index"`
	RemoteID               string `gorm:"not null;default:''"`
	PublishURL             string `gorm:"type:text;not null;default:''"`
	ErrorCode              string `gorm:"not null;default:''"`
	ErrorMessage           string `gorm:"type:text;not null;default:''"`
	CreatedAt              time.Time
	ScheduledPublication   ScheduledPublication `gorm:"foreignKey:ScheduledPublicationID;constraint:OnDelete:CASCADE"`
}

const (
	OutboxAggregatePublishJob = "publish_job"

	OutboxEventPublishJobRequested = "publish.job_requested"

	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDispatched = "dispatched"
	OutboxStatusFailed     = "failed"
)

type OutboxEvent struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	AggregateType string         `gorm:"not null;index:idx_outbox_events_dispatch,priority:1"`
	AggregateID   uuid.UUID      `gorm:"type:uuid;not null;index"`
	EventType     string         `gorm:"not null;index"`
	Payload       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Status        string         `gorm:"not null;default:'pending';index:idx_outbox_events_dispatch,priority:2"`
	Attempts      int            `gorm:"not null;default:0"`
	NextAttemptAt *time.Time     `gorm:"index:idx_outbox_events_dispatch,priority:3"`
	ProcessedAt   *time.Time
	ErrorMessage  string `gorm:"type:text;not null;default:''"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PlatformAccount struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	UserID              uuid.UUID      `gorm:"type:uuid;not null;index:idx_platform_accounts_user_platform"`
	WorkspaceID         *uuid.UUID     `gorm:"type:uuid;index:idx_platform_accounts_workspace_platform"`
	OwnerUserID         *uuid.UUID     `gorm:"type:uuid;index"`
	ConnectedByUserID   *uuid.UUID     `gorm:"type:uuid;index"`
	Platform            string         `gorm:"not null;index:idx_platform_accounts_user_platform;index:idx_platform_accounts_workspace_platform;index:idx_platform_accounts_platform_status"`
	Username            string         `gorm:"not null;default:''"`
	DisplayName         string         `gorm:"not null;default:''"`
	PlatformUserID      string         `gorm:"not null;default:'';index"`
	ShareScope          string         `gorm:"not null;default:'private';index"`
	Status              string         `gorm:"not null;default:'untested';index:idx_platform_accounts_platform_status"`
	HealthStatus        string         `gorm:"not null;default:'unknown';index"`
	CredentialSecretRef string         `gorm:"not null;default:''"`
	Cookies             datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Credentials         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Metadata            datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Config              datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	AvatarURL           string
	LastConnectedAt     *time.Time
	LastVerifiedAt      *time.Time
	LastTestedAt        *time.Time
	LastTestError       string
	ExpiresAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type RemoteBrowserSession struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID                uuid.UUID  `gorm:"type:uuid;not null;index:idx_browser_sessions_user_platform"`
	WorkspaceID           *uuid.UUID `gorm:"type:uuid;index"`
	PlatformAccountID     *uuid.UUID `gorm:"type:uuid;index"`
	Platform              string     `gorm:"not null;index:idx_browser_sessions_user_platform"`
	Status                string     `gorm:"not null;index:idx_browser_sessions_user_platform"`
	WorkerSessionRef      string     `gorm:"not null;default:''"`
	ContainerID           string     `gorm:"not null;default:''"`
	CDPEndpointRef        string     `gorm:"not null;default:''"`
	StreamEndpointRef     string     `gorm:"not null;default:''"`
	ConnectTokenHash      string     `gorm:"not null"`
	ConnectTokenExpiresAt time.Time
	ErrorMessage          string    `gorm:"not null;default:''"`
	CreatedAt             time.Time `gorm:"not null"`
	ExpiresAt             time.Time `gorm:"not null"`
	CompletedAt           *time.Time
}

type PlatformAccountGrant struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey"`
	PlatformAccountID uuid.UUID       `gorm:"type:uuid;not null;index:idx_platform_account_grants_account"`
	WorkspaceID       uuid.UUID       `gorm:"type:uuid;not null;index"`
	GranteeUserID     *uuid.UUID      `gorm:"type:uuid;index"`
	ProjectID         *uuid.UUID      `gorm:"type:uuid;index"`
	Role              string          `gorm:"not null;index"`
	CreatedBy         uuid.UUID       `gorm:"type:uuid;not null;index"`
	CreatedAt         time.Time       `gorm:"not null"`
	UpdatedAt         time.Time       `gorm:"not null"`
	Account           PlatformAccount `gorm:"foreignKey:PlatformAccountID;constraint:OnDelete:CASCADE"`
	Workspace         Workspace       `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE"`
}

type ExtensionCallbackToken struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	ExecutionID string    `gorm:"not null;index"`
	ProjectID   uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Platform    string    `gorm:"not null;index"`
	Token       string    `gorm:"not null;uniqueIndex"`
	ExpiresAt   time.Time `gorm:"not null;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ExtensionExecutionEvent struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	CallbackTokenID uuid.UUID `gorm:"type:uuid;not null;index"`
	ExecutionID     string    `gorm:"not null;index"`
	ProjectID       uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	EventID         string    `gorm:"not null;uniqueIndex"`
	Platform        string    `gorm:"not null;index"`
	Status          string    `gorm:"not null;index"`
	Message         string
	RemoteID        string
	PublishURL      string
	ErrorMessage    string
	Metadata        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt       time.Time
}

// BeforeCreate hook to generate UUID if not set
func (u *User) BeforeCreate(_ *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

func (p *Project) BeforeCreate(_ *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

func (m *MediaAsset) BeforeCreate(_ *gorm.DB) (err error) {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = MediaAssetStatusPending
	}
	if m.LibraryScope == "" {
		m.LibraryScope = MediaAssetLibraryScopeProject
	}
	if m.Tags == nil {
		m.Tags = datatypes.JSON([]byte(`[]`))
	}
	return
}

func (t *ContentTemplate) BeforeCreate(_ *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.Scope == "" {
		t.Scope = ContentTemplateScopePersonal
	}
	if t.DefaultPlatforms == nil {
		t.DefaultPlatforms = datatypes.JSON([]byte(`[]`))
	}
	if t.PlatformConfig == nil {
		t.PlatformConfig = datatypes.JSON([]byte(`{}`))
	}
	if t.Tags == nil {
		t.Tags = datatypes.JSON([]byte(`[]`))
	}
	return
}

func (b *BrandProfile) BeforeCreate(_ *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.BannedWords == nil {
		b.BannedWords = datatypes.JSON([]byte(`[]`))
	}
	if b.DefaultTags == nil {
		b.DefaultTags = datatypes.JSON([]byte(`[]`))
	}
	return
}

func (u *MediaAssetUsage) BeforeCreate(_ *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

func (p *ProjectPlatformPublication) BeforeCreate(_ *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.DraftStatus == "" {
		p.DraftStatus = PublicationDraftStatusUnsynced
	}
	if p.ReviewStatus == "" {
		p.ReviewStatus = PublicationReviewStatusDraft
	}
	return
}

func (e *PublishEvent) BeforeCreate(_ *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return
}

func (s *ScheduledPublication) BeforeCreate(_ *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Status == "" {
		s.Status = ScheduledPublicationStatusScheduled
	}
	if s.Timezone == "" {
		s.Timezone = "UTC"
	}
	return
}

func (a *PublishAttempt) BeforeCreate(_ *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Status == "" {
		a.Status = PublishAttemptStatusRunning
	}
	return
}

func (e *OutboxEvent) BeforeCreate(_ *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Payload == nil {
		e.Payload = datatypes.JSON([]byte(`{}`))
	}
	if e.Status == "" {
		e.Status = OutboxStatusPending
	}
	return
}

func (w *Workspace) BeforeCreate(_ *gorm.DB) (err error) {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	if w.Status == "" {
		w.Status = WorkspaceStatusActive
	}
	return
}

func (a *WorkspaceActivity) BeforeCreate(_ *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return
}

func (n *Notification) BeforeCreate(_ *gorm.DB) (err error) {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.Status == "" {
		n.Status = NotificationStatusUnread
	}
	return
}

func (i *WorkspaceInvite) BeforeCreate(_ *gorm.DB) (err error) {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Status == "" {
		i.Status = WorkspaceInviteStatusPending
	}
	return
}

func (a *ProjectActivity) BeforeCreate(_ *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return
}

func (c *ProjectComment) BeforeCreate(_ *gorm.DB) (err error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.Status == "" {
		c.Status = ProjectCommentStatusOpen
	}
	return
}

func (v *ProjectVersion) BeforeCreate(_ *gorm.DB) (err error) {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return
}

func (l *ProjectShareLink) BeforeCreate(_ *gorm.DB) (err error) {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.Status == "" {
		l.Status = ProjectShareLinkStatusActive
	}
	return
}

func (pa *PlatformAccount) BeforeCreate(_ *gorm.DB) (err error) {
	if pa.ID == uuid.Nil {
		pa.ID = uuid.New()
	}
	if pa.OwnerUserID == nil && pa.UserID != uuid.Nil {
		ownerID := pa.UserID
		pa.OwnerUserID = &ownerID
	}
	if pa.ConnectedByUserID == nil && pa.UserID != uuid.Nil {
		connectedBy := pa.UserID
		pa.ConnectedByUserID = &connectedBy
	}
	if pa.WorkspaceID == nil && pa.UserID != uuid.Nil {
		workspaceID := PersonalWorkspaceID(pa.UserID)
		pa.WorkspaceID = &workspaceID
	}
	if pa.DisplayName == "" {
		pa.DisplayName = pa.Username
	}
	if pa.ShareScope == "" {
		pa.ShareScope = PlatformAccountSharePrivate
	}
	if pa.HealthStatus == "" {
		pa.HealthStatus = PlatformAccountHealthUnknown
	}
	if pa.CredentialSecretRef == "" {
		pa.CredentialSecretRef = "platform-account:" + pa.ID.String()
	}
	return
}

func (s *RemoteBrowserSession) BeforeCreate(_ *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return
}

func (g *PlatformAccountGrant) BeforeCreate(_ *gorm.DB) (err error) {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return
}

func (t *ExtensionCallbackToken) BeforeCreate(_ *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return
}

func (e *ExtensionExecutionEvent) BeforeCreate(_ *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return
}
