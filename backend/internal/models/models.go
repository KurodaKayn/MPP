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

type User struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Username  string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Projects  []Project `gorm:"foreignKey:UserID"`
}

type Project struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index:idx_projects_user_status_created_at"`
	Title         string    `gorm:"not null"`
	SourceContent string    `gorm:"type:text;not null"`
	Status        string    `gorm:"not null;index:idx_projects_user_status_created_at;index:idx_projects_status_created_at"`
	CreatedAt     time.Time `gorm:"index:idx_projects_user_status_created_at;index:idx_projects_status_created_at"`
	UpdatedAt     time.Time
	Publications  []ProjectPlatformPublication `gorm:"foreignKey:ProjectID"`
}

type ProjectPlatformPublication struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
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
