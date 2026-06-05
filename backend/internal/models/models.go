package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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
	PublicationStatusPending    = string(contracts.PublicationStatusPending)
	PublicationStatusAdapted    = string(contracts.PublicationStatusAdapted)
	PublicationStatusPublishing = string(contracts.PublicationStatusPublishing)
	PublicationStatusPublished  = string(contracts.PublicationStatusPublished)
	PublicationStatusFailed     = string(contracts.PublicationStatusFailed)
	PublicationStatusDisabled   = string(contracts.PublicationStatusDisabled)
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
	ProjectRoleOwner  = "owner"
	ProjectRoleEditor = "editor"
	ProjectRoleViewer = "viewer"
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

const (
	WorkspaceStatusActive   = "active"
	WorkspaceStatusArchived = "archived"

	PersonalWorkspaceName = "Personal"

	WorkspaceRoleOwner  = "owner"
	WorkspaceRoleAdmin  = "admin"
	WorkspaceRoleMember = "member"
	WorkspaceRoleViewer = "viewer"
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
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

func (p *Project) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

func (p *ProjectPlatformPublication) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

func (w *Workspace) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	if w.Status == "" {
		w.Status = WorkspaceStatusActive
	}
	return
}

func (pa *PlatformAccount) BeforeCreate(tx *gorm.DB) (err error) {
	if pa.ID == uuid.Nil {
		pa.ID = uuid.New()
	}
	return
}

func (s *RemoteBrowserSession) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return
}

func (t *ExtensionCallbackToken) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return
}

func (e *ExtensionExecutionEvent) BeforeCreate(tx *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return
}
