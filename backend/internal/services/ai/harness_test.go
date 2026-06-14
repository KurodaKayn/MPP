package ai

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestEventLogManagerAndReplay(t *testing.T) {
	db := testsupport.SetupTestDB()
	err := db.AutoMigrate(
		&models.AIDraftingSession{},
		&models.AISessionEvent{},
	)
	require.NoError(t, err)

	sessionID := uuid.New()
	eventMgr := NewEventLogManager(db)
	ctx := context.Background()

	// Log some timeline events
	_, err = eventMgr.LogEvent(ctx, sessionID, "status", map[string]string{"state": "started"})
	require.NoError(t, err)

	_, err = eventMgr.LogEvent(ctx, sessionID, "tool_call", map[string]string{"tool": "read_draft"})
	require.NoError(t, err)

	_, err = eventMgr.LogEvent(ctx, sessionID, "message", map[string]string{"text": "Hello"})
	require.NoError(t, err)

	// Replay and verify
	timeline, err := eventMgr.Replay(ctx, sessionID)
	require.NoError(t, err)

	require.Len(t, timeline, 3)
	assert.Equal(t, "status", timeline[0].EventType)
	assert.Equal(t, "started", timeline[0].Payload["state"])

	assert.Equal(t, "tool_call", timeline[1].EventType)
	assert.Equal(t, "read_draft", timeline[1].Payload["tool"])

	assert.Equal(t, "message", timeline[2].EventType)
	assert.Equal(t, "Hello", timeline[2].Payload["text"])
}

func TestRunnerStateMachineLoop(t *testing.T) {
	db := testsupport.SetupTestDB()
	err := db.AutoMigrate(
		&models.AIContextSnapshot{},
		&models.AIGrowthOptimizationRun{},
		&models.AIProposal{},
		&models.AIDraftingSession{},
		&models.AIDraftingMessage{},
		&models.AIToolCall{},
		&models.AIDraftingSessionSummary{},
		&models.AISessionEvent{},
		&models.AIUsageRecord{},
		&models.WorkspaceQuotaAggregate{},
		&models.MediaAsset{},
		&models.ProjectComment{},
	)
	require.NoError(t, err)

	userID := uuid.New()
	projectID := uuid.New()
	sessionID := uuid.New()

	// Seed project
	project := models.Project{
		ID:            projectID,
		UserID:        userID,
		Title:         "Event Loop Project",
		SourceContent: "Original content to optimize",
		Status:        "draft",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&project).Error)

	// Seed session
	session := models.AIDraftingSession{
		ID:            sessionID,
		WorkspaceID:   uuid.New(),
		ProjectID:     projectID,
		CreatedByID:   userID,
		Title:         "Optimization Session",
		Status:        "active",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	assembler := NewAIContextAssembler(db)
	eventMgr := NewEventLogManager(db)
	quotaSvc := NewQuotaService(db)
	runner := NewRunner(db, eventMgr, assembler, quotaSvc)

	// Register a mock tool
	toolExecuted := false
	mockTool := NewMockTool("read_project_context", "Read project context", func(ctx context.Context, args json.RawMessage) (string, error) {
		toolExecuted = true
		return "Mock tool executed successfully returning project data", nil
	})
	runner.RegisterTool(mockTool)

	sseChan := make(chan string, 20)
	ctx := context.Background()

	err = runner.RunSession(ctx, sessionID, userID, "Please optimize my drafting content", sseChan)
	require.NoError(t, err)

	close(sseChan)

	// Assert tool was run
	assert.True(t, toolExecuted)

	// Check that final proposals exist in DB
	var proposals []models.AIProposal
	err = db.Where("session_id = ?", sessionID).Find(&proposals).Error
	require.NoError(t, err)
	require.Len(t, proposals, 1)
	assert.Equal(t, "source_rewrite", proposals[0].ProposalType)
	assert.Equal(t, "wechat", proposals[0].TargetPlatform)

	// Check logged events in DB
	events, err := eventMgr.GetEvents(ctx, sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, events)

	// Verify that at least one "tool_call", "tool_result", "proposal" and "message" events were written
	hasToolCall := false
	hasToolResult := false
	hasProposal := false
	hasStatus := false

	for _, e := range events {
		switch e.EventType {
		case "tool_call":
			hasToolCall = true
		case "tool_result":
			hasToolResult = true
		case "proposal":
			hasProposal = true
		case "status":
			hasStatus = true
		}
	}

	assert.True(t, hasToolCall, "timeline missing tool_call event")
	assert.True(t, hasToolResult, "timeline missing tool_result event")
	assert.True(t, hasProposal, "timeline missing proposal event")
	assert.True(t, hasStatus, "timeline missing status event")
}

func TestRunnerWritesStructuredErrorEventOnToolFailure(t *testing.T) {
	db := testsupport.SetupTestDB()
	err := db.AutoMigrate(
		&models.AIContextSnapshot{},
		&models.AIProposal{},
		&models.AIDraftingSession{},
		&models.AIDraftingMessage{},
		&models.AIToolCall{},
		&models.AISessionEvent{},
		&models.AIUsageRecord{},
		&models.WorkspaceQuotaAggregate{},
		&models.MediaAsset{},
		&models.ProjectComment{},
	)
	require.NoError(t, err)

	userID := uuid.New()
	projectID := uuid.New()
	sessionID := uuid.New()
	project := models.Project{
		ID:            projectID,
		UserID:        userID,
		Title:         "Failure Project",
		SourceContent: "Original content",
		Status:        "draft",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&project).Error)
	session := models.AIDraftingSession{
		ID:            sessionID,
		WorkspaceID:   uuid.New(),
		ProjectID:     projectID,
		CreatedByID:   userID,
		Title:         "Failure Session",
		Status:        "active",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	assembler := NewAIContextAssembler(db)
	eventMgr := NewEventLogManager(db)
	runner := NewRunnerWithAdapter(db, eventMgr, assembler, NewQuotaService(db), unknownToolAdapter{})

	err = runner.RunSession(context.Background(), sessionID, userID, "Please optimize", nil)
	require.Error(t, err)

	timeline, err := eventMgr.Replay(context.Background(), sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, timeline)
	last := timeline[len(timeline)-1]
	assert.Equal(t, EventError, last.EventType)
	assert.Equal(t, "missing_tool", last.Payload["code"])
	assert.Contains(t, last.Payload["message"], "unknown_tool")
}

func TestRunnerWritesStructuredErrorEventsForPhase0DFailures(t *testing.T) {
	tests := []struct {
		name      string
		adapter   LLMAdapter
		register  func(*Runner)
		wantCode  string
		wantError string
	}{
		{
			name:      "schema error",
			adapter:   toolCallAdapter{toolName: "read_project_context", arguments: []byte(`{`)},
			wantCode:  "schema_error",
			wantError: "invalid arguments",
		},
		{
			name:    "denied permission",
			adapter: toolCallAdapter{toolName: "permission_tool", arguments: []byte(`{}`)},
			register: func(r *Runner) {
				r.RegisterTool(NewMockTool("permission_tool", "Denied tool", func(ctx context.Context, args json.RawMessage) (string, error) {
					return "", ErrToolPermissionDenied
				}))
			},
			wantCode:  "permission_denied",
			wantError: ErrToolPermissionDenied.Error(),
		},
		{
			name:      "timeout",
			adapter:   failingAdapter{err: context.DeadlineExceeded},
			wantCode:  "timeout",
			wantError: context.DeadlineExceeded.Error(),
		},
		{
			name:      "user interruption",
			adapter:   failingAdapter{err: context.Canceled},
			wantCode:  "user_interrupted",
			wantError: context.Canceled.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, userID, sessionID := setupHarnessRun(t)
			assembler := NewAIContextAssembler(db)
			eventMgr := NewEventLogManager(db)
			runner := NewRunnerWithAdapter(db, eventMgr, assembler, NewQuotaService(db), tt.adapter)
			if tt.register != nil {
				tt.register(runner)
			}

			err := runner.RunSession(context.Background(), sessionID, userID, "Please optimize", nil)
			require.Error(t, err)

			replay, err := eventMgr.ReplaySession(context.Background(), sessionID)
			require.NoError(t, err)
			require.NotEmpty(t, replay.Events)
			last := replay.Events[len(replay.Events)-1]
			assert.Equal(t, EventError, last.EventType)
			assert.Equal(t, tt.wantCode, last.Payload["code"])
			assert.Contains(t, last.Payload["message"], tt.wantError)
		})
	}
}

func TestReplaySessionIncludesModelVisibleMessageSequence(t *testing.T) {
	db, userID, sessionID := setupHarnessRun(t)
	assembler := NewAIContextAssembler(db)
	eventMgr := NewEventLogManager(db)
	runner := NewRunner(db, eventMgr, assembler, NewQuotaService(db))

	err := runner.RunSession(context.Background(), sessionID, userID, "Please optimize my drafting content", nil)
	require.NoError(t, err)

	replay, err := eventMgr.ReplaySession(context.Background(), sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, replay.Events)
	require.Len(t, replay.ModelMessages, 3)
	assert.Equal(t, "user", replay.ModelMessages[0].Role)
	assert.Equal(t, "Please optimize my drafting content", replay.ModelMessages[0].Content)
	assert.Equal(t, "assistant", replay.ModelMessages[1].Role)
	assert.Equal(t, "assistant", replay.ModelMessages[2].Role)
}

func setupHarnessRun(t *testing.T) (*gorm.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := testsupport.SetupTestDB()
	err := db.AutoMigrate(
		&models.AIContextSnapshot{},
		&models.AIProposal{},
		&models.AIDraftingSession{},
		&models.AIDraftingMessage{},
		&models.AIToolCall{},
		&models.AISessionEvent{},
		&models.AIUsageRecord{},
		&models.WorkspaceQuotaAggregate{},
		&models.MediaAsset{},
		&models.ProjectComment{},
	)
	require.NoError(t, err)

	userID := uuid.New()
	projectID := uuid.New()
	sessionID := uuid.New()
	project := models.Project{
		ID:            projectID,
		UserID:        userID,
		Title:         "Phase 0D Project",
		SourceContent: "Original content",
		Status:        "draft",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&project).Error)
	session := models.AIDraftingSession{
		ID:            sessionID,
		WorkspaceID:   uuid.New(),
		ProjectID:     projectID,
		CreatedByID:   userID,
		Title:         "Phase 0D Session",
		Status:        "active",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	return db, userID, sessionID
}

type unknownToolAdapter struct{}

func (unknownToolAdapter) Query(_ context.Context, sessionID uuid.UUID, _ []models.AIDraftingMessage, _ *models.AIContextSnapshot, _ []models.AIToolCall) (*LLMResponse, error) {
	return &LLMResponse{
		ToolCalls: []models.AIToolCall{
			{
				ID:        uuid.New(),
				SessionID: sessionID,
				ToolName:  "unknown_tool",
				Version:   "1.0",
				Arguments: []byte(`{}`),
			},
		},
	}, nil
}

type toolCallAdapter struct {
	toolName  string
	arguments []byte
}

func (a toolCallAdapter) Query(_ context.Context, sessionID uuid.UUID, _ []models.AIDraftingMessage, _ *models.AIContextSnapshot, _ []models.AIToolCall) (*LLMResponse, error) {
	return &LLMResponse{
		ToolCalls: []models.AIToolCall{
			{
				ID:        uuid.New(),
				SessionID: sessionID,
				ToolName:  a.toolName,
				Version:   "1.0",
				Arguments: a.arguments,
			},
		},
	}, nil
}

type failingAdapter struct {
	err error
}

func (a failingAdapter) Query(_ context.Context, _ uuid.UUID, _ []models.AIDraftingMessage, _ *models.AIContextSnapshot, _ []models.AIToolCall) (*LLMResponse, error) {
	return nil, a.err
}
