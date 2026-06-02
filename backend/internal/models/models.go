package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Project Status Constants
const (
	ProjectStatusDraft      = "draft"
	ProjectStatusReady      = "ready"
	ProjectStatusPublishing = "publishing"
	ProjectStatusPublished  = "published"
	ProjectStatusFailed     = "failed"
)

// Publication Status Constants
const (
	PublicationStatusPending    = "pending"
	PublicationStatusAdapted    = "adapted"
	PublicationStatusPublishing = "publishing"
	PublicationStatusPublished  = "published"
	PublicationStatusFailed     = "failed"
	PublicationStatusDisabled   = "disabled"
)

// Platform account status constants
const (
	PlatformAccountStatusUntested  = "untested"
	PlatformAccountStatusConnected = "connected"
	PlatformAccountStatusFailed    = "failed"
)

// Remote Browser Session Status Constants
const (
	BrowserSessionStatusPending       = "pending"
	BrowserSessionStatusReady         = "ready"
	BrowserSessionStatusLoginDetected = "login_detected"
	BrowserSessionStatusCapturing     = "capturing"
	BrowserSessionStatusConnected     = "connected"
	BrowserSessionStatusExpired       = "expired"
	BrowserSessionStatusFailed        = "failed"
)

type User struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Username              string    `gorm:"not null;uniqueIndex"`
	PasswordHash          string    `gorm:"not null"`
	Role                  string    `gorm:"not null;default:'user'"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Projects              []Project              `gorm:"foreignKey:UserID"`
	PlatformAccounts      []PlatformAccount      `gorm:"foreignKey:UserID"`
	RemoteBrowserSessions []RemoteBrowserSession `gorm:"foreignKey:UserID"`
}

type Project struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index:idx_projects_user_status_created_at"`
	Title         string    `gorm:"not null"`
	SourceContent string    `gorm:"type:text;not null"`
	Status        string    `gorm:"not null;index:idx_projects_user_status_created_at;index:idx_projects_status_created_at"`
	CreatedAt     time.Time `gorm:"index:idx_projects_user_status_created_at;index:idx_projects_status_created_at"`
	UpdatedAt     time.Time
	Publications  []ProjectPlatformPublication `gorm:"foreignKey:ProjectID"`
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
