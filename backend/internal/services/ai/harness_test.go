package ai

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	mockTool := NewMockTool("read_project_draft", "Read draft details", func(ctx context.Context, args json.RawMessage) (string, error) {
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
