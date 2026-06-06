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
	PlatformAccountStatusUntested  = string(contracts.PlatformAccountStatusUntested)
	PlatformAccountStatusConnected = string(contracts.PlatformAccountStatusConnected)
	PlatformAccountStatusFailed    = string(contracts.PlatformAccountStatusFailed)
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
	Title            string     `gorm:"not null"`
	SourceContent    string     `gorm:"type:text;not null"`
	Status           string     `gorm:"not null;index:idx_projects_user_status_created_at;index:idx_projects_status_created_at;index:idx_projects_workspace_status_created_at"`
	CreatedAt        time.Time  `gorm:"index:idx_projects_user_status_created_at;index:idx_projects_status_created_at;index:idx_projects_workspace_status_created_at"`
	UpdatedAt        time.Time
	Publications     []ProjectPlatformPublication `gorm:"foreignKey:ProjectID"`
	Collaborators    []ProjectCollaborator        `gorm:"foreignKey:ProjectID"`
	CollabDocument   *CollabDocument              `gorm:"foreignKey:CollabDocumentID;references:ID;constraint:OnDelete:SET NULL"`
	Workspace        *Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:SET NULL"`
}

const (
	MediaAssetStatusPending = "pending"
	MediaAssetStatusReady   = "ready"
	MediaAssetStatusFailed  = "failed"
	MediaAssetStatusDeleted = "deleted"

	MediaAssetUsageEditorImage = "editor_image"
	MediaAssetUsageCoverImage  = "cover_image"
)

type MediaAsset struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	UserID           uuid.UUID      `gorm:"type:uuid;not null;index"`
	WorkspaceID      *uuid.UUID     `gorm:"type:uuid;index"`
	ProjectID        *uuid.UUID     `gorm:"type:uuid;index"`
	Bucket           string         `gorm:"not null"`
	ObjectKey        string         `gorm:"not null;uniqueIndex"`
	OriginalFilename string         `gorm:"not null"`
	MimeType         string         `gorm:"not null;index"`
	SizeBytes        int64          `gorm:"not null"`
	Usage            string         `gorm:"not null;index"`
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

const (
	ProjectRoleOwner  = "owner"
	ProjectRoleEditor = "editor"
	ProjectRoleViewer = "viewer"
)

const (
	ProjectActivityContentSaved            = "content_saved"
	ProjectActivityCommentCreated          = "comment_created"
	ProjectActivityCommentResolved         = "comment_resolved"
	ProjectActivityCollaboratorAdded       = "collaborator_added"
	ProjectActivityCollaboratorRoleChanged = "collaborator_role_changed"
	ProjectActivityCollaboratorRemoved     = "collaborator_removed"
	ProjectActivityPublishRequested        = "publish_requested"
	ProjectActivityPublishQueued           = "publish_queued"
	ProjectActivityShareLinkAccepted       = "share_link_accepted"
	ProjectActivityShareLinkCreated        = "share_link_created"
	ProjectActivityShareLinkRevoked        = "share_link_revoked"
	ProjectActivityVersionRestored         = "version_restored"
)

const (
	ProjectCommentStatusOpen     = "open"
	ProjectCommentStatusResolved = "resolved"
)

const (
	ProjectShareLinkStatusActive  = "active"
	ProjectShareLinkStatusRevoked = "revoked"
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

type ProjectPlatformPublication struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID      uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_publications_project_platform"`
	Platform       string         `gorm:"not null;uniqueIndex:idx_publications_project_platform;index:idx_publications_platform_status"`
	Enabled        bool           `gorm:"not null;default:true"`
	Status         string         `gorm:"not null;index:idx_publications_platform_status"`
	Config         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	AdaptedContent datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	RemoteID       string
	PublishURL     string
	ErrorMessage   string
	RetryCount     int `gorm:"not null;default:0"`
	LastAttemptAt  *time.Time
	PublishedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
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

type PlatformAccount struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_platform_accounts_user_platform"`
	Platform      string         `gorm:"not null;uniqueIndex:idx_platform_accounts_user_platform;index:idx_platform_accounts_platform_status"`
	Username      string         `gorm:"not null;default:''"`
	Status        string         `gorm:"not null;default:'untested';index:idx_platform_accounts_platform_status"`
	Cookies       datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"` // From feature branch
	Credentials   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"` // From main branch
	Metadata      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"` // From main branch
	Config        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"` // From feature branch
	AvatarURL     string         // From feature branch
	LastTestedAt  *time.Time
	LastTestError string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RemoteBrowserSession struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID                uuid.UUID `gorm:"type:uuid;not null;index:idx_browser_sessions_user_platform"`
	Platform              string    `gorm:"not null;index:idx_browser_sessions_user_platform"`
	Status                string    `gorm:"not null;index:idx_browser_sessions_user_platform"`
	WorkerSessionRef      string    `gorm:"not null;default:''"`
	ContainerID           string    `gorm:"not null;default:''"`
	CDPEndpointRef        string    `gorm:"not null;default:''"`
	StreamEndpointRef     string    `gorm:"not null;default:''"`
	ConnectTokenHash      string    `gorm:"not null"`
	ConnectTokenExpiresAt time.Time
	ErrorMessage          string    `gorm:"not null;default:''"`
	CreatedAt             time.Time `gorm:"not null"`
	ExpiresAt             time.Time `gorm:"not null"`
	CompletedAt           *time.Time
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
	return
}

func (p *ProjectPlatformPublication) BeforeCreate(_ *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

func (e *PublishEvent) BeforeCreate(_ *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
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
	return
}

func (s *RemoteBrowserSession) BeforeCreate(_ *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
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
