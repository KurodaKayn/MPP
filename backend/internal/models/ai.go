package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type AIContextSnapshot struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProjectID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	CreatedByID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	ContextKind        string     `gorm:"not null"` // "growth_optimization", "drafting"
	SourceVersionID    *uuid.UUID `gorm:"type:uuid;index"`
	ProjectSummary     string     `gorm:"type:text"`
	SourceContent      string     `gorm:"type:text;not null"`
	SelectedRange      string     `gorm:"type:text"`
	Platforms          datatypes.JSON
	Publications       datatypes.JSON
	BrandProfile       datatypes.JSON
	ContentTemplate    datatypes.JSON
	CommentsSummary    string `gorm:"type:text"`
	VersionsSummary    string `gorm:"type:text"`
	MediaSummary       string `gorm:"type:text"`
	PerformanceSummary string `gorm:"type:text"`
	TokenEstimate      int    `gorm:"not null"`
	ContextBudget      int    `gorm:"not null"`
	CompactionLevel    string `gorm:"not null"` // "none", "partial", "session_summary", "memory_summary"
	RawContextRefs     datatypes.JSON
	CreatedAt          time.Time `gorm:"not null;index"`

	// Relationships
	Workspace      *Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
	Project        *Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	CreatedBy      *User           `gorm:"foreignKey:CreatedByID;references:ID;constraint:OnDelete:CASCADE"`
	ProjectVersion *ProjectVersion `gorm:"foreignKey:SourceVersionID;references:ID;constraint:OnDelete:SET NULL"`
}

type AIGrowthOptimizationRun struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null;index"`
	ProjectID         uuid.UUID      `gorm:"type:uuid;not null;index"`
	ContextSnapshotID uuid.UUID      `gorm:"type:uuid;not null;index"`
	Goal              string         `gorm:"not null"`
	Intensity         string         `gorm:"not null"` // "conservative", "balanced", "aggressive"
	TargetPlatforms   datatypes.JSON `gorm:"not null"`
	Status            string         `gorm:"not null;index"` // "running", "ready", "applied", "failed", "cancelled"
	Model             string         `gorm:"not null"`
	PromptVersion     string         `gorm:"not null"`
	Usage             datatypes.JSON
	QualitySummary    string    `gorm:"type:text"`
	CreatedByID       uuid.UUID `gorm:"type:uuid;not null;index"`
	CreatedAt         time.Time `gorm:"not null;index"`
	UpdatedAt         time.Time `gorm:"not null"`

	// Relationships
	Workspace       *Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
	Project         *Project           `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	ContextSnapshot *AIContextSnapshot `gorm:"foreignKey:ContextSnapshotID;references:ID;constraint:OnDelete:RESTRICT"`
	CreatedBy       *User              `gorm:"foreignKey:CreatedByID;references:ID;constraint:OnDelete:CASCADE"`
}

type AIProposal struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProjectID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	SessionID         *uuid.UUID `gorm:"type:uuid;index"`
	RunID             *uuid.UUID `gorm:"type:uuid;index"`
	ContextSnapshotID uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProposalType      string     `gorm:"not null"` // "source_rewrite", "title_candidates", "prepublish_patch", "comment_reply", "checklist", "tag_candidates"
	TargetPlatform    string     `gorm:"not null"`
	Status            string     `gorm:"not null;index"` // "proposed", "accepted", "rejected", "superseded"
	Summary           string     `gorm:"type:text"`
	Patch             string     `gorm:"type:text"`
	FullContent       string     `gorm:"type:text"`
	QualityChecks     datatypes.JSON
	CreatedAt         time.Time `gorm:"not null;index"`
	DecidedAt         *time.Time
	DecidedByID       *uuid.UUID `gorm:"type:uuid;index"`

	// Relationships
	Workspace       *Workspace               `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
	Project         *Project                 `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	Session         *AIDraftingSession       `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:SET NULL"`
	Run             *AIGrowthOptimizationRun `gorm:"foreignKey:RunID;references:ID;constraint:OnDelete:SET NULL"`
	ContextSnapshot *AIContextSnapshot       `gorm:"foreignKey:ContextSnapshotID;references:ID;constraint:OnDelete:RESTRICT"`
	DecidedBy       *User                    `gorm:"foreignKey:DecidedByID;references:ID;constraint:OnDelete:SET NULL"`
}

type AIDraftingSession struct {
	ID                      uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID             uuid.UUID  `gorm:"type:uuid;not null;index"`
	ProjectID               uuid.UUID  `gorm:"type:uuid;not null;index"`
	CreatedByID             uuid.UUID  `gorm:"type:uuid;not null;index"`
	Title                   string     `gorm:"not null"`
	Status                  string     `gorm:"not null;index"` // "active", "archived"
	ActiveContextSnapshotID *uuid.UUID `gorm:"type:uuid;index"`
	LastMessageAt           time.Time  `gorm:"not null;index"`
	CreatedAt               time.Time  `gorm:"not null;index"`
	UpdatedAt               time.Time  `gorm:"not null"`

	// Relationships
	Workspace      *Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
	Project        *Project           `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	CreatedBy      *User              `gorm:"foreignKey:CreatedByID;references:ID;constraint:OnDelete:CASCADE"`
	ActiveSnapshot *AIContextSnapshot `gorm:"foreignKey:ActiveContextSnapshotID;references:ID;constraint:OnDelete:SET NULL"`
}

type AIDraftingMessage struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	SessionID uuid.UUID `gorm:"type:uuid;not null;index"`
	Role      string    `gorm:"not null"` // "user", "assistant", "system"
	Content   string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null;index"`

	// Relationships
	Session *AIDraftingSession `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE"`
}

type AIToolCall struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey"`
	SessionID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	ToolUseID  string         `gorm:"not null;index"`
	ToolName   string         `gorm:"not null"`
	Version    string         `gorm:"not null"`
	Arguments  datatypes.JSON `gorm:"not null"`
	Result     string         `gorm:"type:text"`
	Error      string         `gorm:"type:text"`
	DurationMs int            `gorm:"not null"`
	CreatedAt  time.Time      `gorm:"not null;index"`

	// Relationships
	Session *AIDraftingSession `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE"`
}

type AIDraftingSessionSummary struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	SessionID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	Summary            string    `gorm:"type:text;not null"`
	UserIntent         string    `gorm:"type:text"`
	AcceptedChanges    datatypes.JSON
	RejectedDirections datatypes.JSON
	OpenTasks          datatypes.JSON
	ActiveArtifacts    datatypes.JSON
	SourceRefs         datatypes.JSON
	NextStepHint       string    `gorm:"type:text"`
	CreatedAt          time.Time `gorm:"not null;index"`
	UpdatedAt          time.Time `gorm:"not null"`

	// Relationships
	Session *AIDraftingSession `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE"`
}

type AISessionEvent struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	SessionID    uuid.UUID      `gorm:"type:uuid;not null;index:idx_ai_session_events_sequence,priority:1"`
	Sequence     uint64         `gorm:"not null;index:idx_ai_session_events_sequence,priority:2"`
	TurnID       *uuid.UUID     `gorm:"type:uuid;index"`
	ToolUseID    string         `gorm:"index"`
	EventType    string         `gorm:"not null"` // e.g. "status", "tool_call", "tool_result", "proposal", "message"
	ModelVisible bool           `gorm:"not null"`
	Payload      datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedAt    time.Time      `gorm:"not null;index"`

	// Relationships
	Session *AIDraftingSession `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE"`
}
