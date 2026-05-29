package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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
	UserID        uuid.UUID `gorm:"type:uuid;not null"`
	Title         string    `gorm:"not null"`
	SourceContent string    `gorm:"type:text;not null"`
	Status        string    `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Publications  []ProjectPlatformPublication `gorm:"foreignKey:ProjectID"`
}

type ProjectPlatformPublication struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ProjectID      uuid.UUID      `gorm:"type:uuid;not null"`
	Platform       string         `gorm:"not null"`
	Enabled        bool           `gorm:"not null;default:true"`
	Status         string         `gorm:"not null"`
	Config         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	AdaptedContent datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	RemoteID       string
	PublishURL     string
	ErrorMessage   string
	RetryCount     int            `gorm:"not null;default:0"`
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
