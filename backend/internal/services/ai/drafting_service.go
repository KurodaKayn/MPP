package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var ErrInvalidDraftingRequest = errors.New("invalid drafting request")

type DraftingService struct {
	db       *gorm.DB
	runner   *Runner
	eventMgr *EventLogManager
}

type StartDraftingSessionRequest struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Message   string
	Title     string
}

type ContinueDraftingSessionRequest struct {
	SessionID uuid.UUID
	UserID    uuid.UUID
	Message   string
}

func NewDraftingService(db *gorm.DB) *DraftingService {
	eventMgr := NewEventLogManager(db)
	assembler := NewAIContextAssembler(db)
	quotaSvc := NewQuotaService(db)
	runner := NewRunner(db, eventMgr, assembler, quotaSvc)
	return &DraftingService{db: db, runner: runner, eventMgr: eventMgr}
}

func NewDraftingServiceWithRunner(db *gorm.DB, runner *Runner, eventMgr *EventLogManager) *DraftingService {
	return &DraftingService{db: db, runner: runner, eventMgr: eventMgr}
}

func (s *DraftingService) Start(ctx context.Context, req StartDraftingSessionRequest, stream chan<- string) (*models.AIDraftingSession, error) {
	if req.ProjectID == uuid.Nil || req.UserID == uuid.Nil || strings.TrimSpace(req.Message) == "" {
		return nil, ErrInvalidDraftingRequest
	}
	var project models.Project
	if err := s.db.WithContext(ctx).Select("id", "workspace_id", "user_id", "title").First(&project, "id = ?", req.ProjectID).Error; err != nil {
		return nil, err
	}
	workspaceID := models.PersonalWorkspaceID(project.UserID)
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		workspaceID = *project.WorkspaceID
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Drafting: " + strings.TrimSpace(project.Title)
	}
	if title == "Drafting:" {
		title = "Drafting Session"
	}
	now := time.Now()
	session := models.AIDraftingSession{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		ProjectID:     req.ProjectID,
		CreatedByID:   req.UserID,
		Title:         title,
		Status:        "active",
		LastMessageAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return nil, err
	}
	if err := s.runner.RunSession(ctx, session.ID, req.UserID, req.Message, stream); err != nil {
		return &session, err
	}
	return &session, nil
}

func (s *DraftingService) Continue(ctx context.Context, req ContinueDraftingSessionRequest, stream chan<- string) (*models.AIDraftingSession, error) {
	if req.SessionID == uuid.Nil || req.UserID == uuid.Nil || strings.TrimSpace(req.Message) == "" {
		return nil, ErrInvalidDraftingRequest
	}
	var session models.AIDraftingSession
	if err := s.db.WithContext(ctx).First(&session, "id = ?", req.SessionID).Error; err != nil {
		return nil, err
	}
	if err := s.runner.RunSession(ctx, session.ID, req.UserID, req.Message, stream); err != nil {
		return &session, err
	}
	return &session, nil
}

func (s *DraftingService) Replay(ctx context.Context, sessionID uuid.UUID) (*ReplayResult, error) {
	return s.eventMgr.ReplaySession(ctx, sessionID)
}

func (s *DraftingService) GetSession(ctx context.Context, sessionID uuid.UUID) (*models.AIDraftingSession, error) {
	var session models.AIDraftingSession
	if err := s.db.WithContext(ctx).First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

type ReadProjectContextTool struct {
	assembler   *AIContextAssembler
	projectID   uuid.UUID
	createdByID uuid.UUID
}

func NewReadProjectContextTool(assembler *AIContextAssembler) *ReadProjectContextTool {
	return &ReadProjectContextTool{assembler: assembler}
}

func NewReadProjectContextToolForProject(assembler *AIContextAssembler, projectID uuid.UUID, createdByID uuid.UUID) *ReadProjectContextTool {
	return &ReadProjectContextTool{assembler: assembler, projectID: projectID, createdByID: createdByID}
}

func (t *ReadProjectContextTool) Name() string {
	return "read_project_context"
}

func (t *ReadProjectContextTool) Description() string {
	return "Read the current project drafting context snapshot."
}

func (t *ReadProjectContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var req struct {
		ProjectID string `json:"project_id"`
	}
	_ = json.Unmarshal(args, &req)
	projectID := t.projectID
	if projectID == uuid.Nil && strings.TrimSpace(req.ProjectID) != "" {
		parsed, err := uuid.Parse(req.ProjectID)
		if err != nil {
			return "", fmt.Errorf("%w: invalid project_id", ErrInvalidDraftingRequest)
		}
		projectID = parsed
	}
	if projectID == uuid.Nil || t.createdByID == uuid.Nil {
		return "", fmt.Errorf("%w: project context is not bound", ErrInvalidDraftingRequest)
	}
	snapshot, err := t.assembler.Assemble(ctx, projectID, t.createdByID, "drafting", AssembleOptions{
		ContextBudget: defaultDraftingContextBudget,
	})
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"project_id":          snapshot.ProjectID,
		"workspace_id":        snapshot.WorkspaceID,
		"project_summary":     snapshot.ProjectSummary,
		"source_content":      snapshot.SourceContent,
		"comments_summary":    snapshot.CommentsSummary,
		"versions_summary":    snapshot.VersionsSummary,
		"media_summary":       snapshot.MediaSummary,
		"performance_summary": snapshot.PerformanceSummary,
		"token_estimate":      snapshot.TokenEstimate,
		"compaction_level":    snapshot.CompactionLevel,
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
