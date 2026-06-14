package models

import (
	"time"

	"github.com/google/uuid"
)

// AIUsageRecord captures the real token usage and cost returned by the
// AI provider for a single request. It is written after every successful
// AI call so that usage can be audited and aggregated per workspace.
type AIUsageRecord struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	SessionID    *uuid.UUID `gorm:"type:uuid;index"` // nil for non-session calls (e.g. EditContent)
	CallKind     string     `gorm:"not null"`        // "drafting", "edit_content", "edit_prepublish"
	InputTokens  int64      `gorm:"not null"`
	OutputTokens int64      `gorm:"not null"`
	TotalTokens  int64      `gorm:"not null"`
	Cost         float64    `gorm:"not null"`
	Currency     string     `gorm:"not null"`
	CreatedAt    time.Time  `gorm:"not null;index"`

	// Relationships
	Workspace *Workspace `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
	User      *User      `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}

// WorkspaceQuotaAggregate holds the running token/cost totals for a
// workspace. It is upserted atomically on every AIUsageRecord write.
// Callers check TotalTokens against a configured limit before sending
// a new AI request.
type WorkspaceQuotaAggregate struct {
	WorkspaceID uuid.UUID `gorm:"type:uuid;primaryKey"`
	TotalTokens int64     `gorm:"not null;default:0"`
	TotalCost   float64   `gorm:"not null;default:0"`
	Currency    string    `gorm:"not null;default:'USD'"`
	UpdatedAt   time.Time `gorm:"not null;index"`

	// Relationships
	Workspace *Workspace `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
}
