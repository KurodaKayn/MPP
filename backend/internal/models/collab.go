package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CollabDocumentStatusActive = "active"
)

const (
	CollabDocumentRoleEditor = "editor"
	CollabDocumentRoleViewer = "viewer"
)

type CollabDocument struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	OwnerUserID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_collab_documents_owner_updated,priority:1"`
	Title         string     `gorm:"not null"`
	Status        string     `gorm:"not null;default:'active'"`
	SchemaVersion int        `gorm:"not null;default:1"`
	CurrentSeq    int64      `gorm:"not null;default:0"`
	LastEditedBy  *uuid.UUID `gorm:"type:uuid"`
	LastEditedAt  *time.Time
	CreatedAt     time.Time      `gorm:"not null"`
	UpdatedAt     time.Time      `gorm:"not null;index:idx_collab_documents_owner_updated,priority:2,sort:desc"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`

	Owner         User                         `gorm:"foreignKey:OwnerUserID;references:ID"`
	LastEditor    *User                        `gorm:"foreignKey:LastEditedBy;references:ID"`
	Collaborators []CollabDocumentCollaborator `gorm:"foreignKey:DocumentID"`
}

type CollabDocumentCollaborator struct {
	DocumentID uuid.UUID `gorm:"type:uuid;primaryKey;not null"`
	UserID     uuid.UUID `gorm:"type:uuid;primaryKey;not null;index:idx_collab_document_collaborators_user,priority:1"`
	Role       string    `gorm:"not null;check:role IN ('editor','viewer');index:idx_collab_document_collaborators_user,priority:2"`
	CreatedBy  uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt  time.Time `gorm:"not null"`

	Document CollabDocument `gorm:"foreignKey:DocumentID;references:ID;constraint:OnDelete:CASCADE"`
	User     User           `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	Creator  User           `gorm:"foreignKey:CreatedBy;references:ID"`
}

func (d *CollabDocument) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return
}
