package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

// Tool represents a registered harness tool that the AI agent can execute
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// MockTool is a helper struct to construct tools on the fly
type MockTool struct {
	name string
	desc string
	fn   func(ctx context.Context, args json.RawMessage) (string, error)
}

func NewMockTool(name string, desc string, fn func(ctx context.Context, args json.RawMessage) (string, error)) *MockTool {
	return &MockTool{name: name, desc: desc, fn: fn}
}

func (t *MockTool) Name() string        { return t.name }
func (t *MockTool) Description() string { return t.desc }
func (t *MockTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.fn(ctx, args)
}

// Runner drives the agentic AI harness execution loop
type Runner struct {
	db        *gorm.DB
	eventMgr  *EventLogManager
	assembler *AIContextAssembler
	tools     map[string]Tool
}

func NewRunner(db *gorm.DB, eventMgr *EventLogManager, assembler *AIContextAssembler) *Runner {
	return &Runner{
		db:        db,
		eventMgr:  eventMgr,
		assembler: assembler,
		tools:     make(map[string]Tool),
	}
}

func (r *Runner) RegisterTool(t Tool) {
	r.tools[t.Name()] = t
}

// MockLLMAdapter simulates an agentic LLM making queries, calling tools, and returning proposals
type MockLLMAdapter struct {
	Turn int
}

type LLMResponse struct {
	Message   string              `json:"message,omitempty"`
	ToolCalls []models.AIToolCall `json:"tool_calls,omitempty"`
	Proposals []models.AIProposal `json:"proposals,omitempty"`
}

func (m *MockLLMAdapter) Query(ctx context.Context, sessionID uuid.UUID, messages []models.AIDraftingMessage, snapshot *models.AIContextSnapshot) (*LLMResponse, error) {
	m.Turn++
	switch m.Turn {
	case 1:
		// First turn: requests a tool call to read the draft
		toolCallID := uuid.New()
		return &LLMResponse{
			Message: "Let me check the project details first.",
			ToolCalls: []models.AIToolCall{
				{
					ID:        toolCallID,
					SessionID: sessionID,
					ToolName:  "read_project_draft",
					Version:   "1.0",
					Arguments: datatypes.JSON(`{"project_id": "` + snapshot.ProjectID.String() + `"}`),
				},
			},
		}, nil
	default:
		// Subsequent turns: returns final assistant response and proposal content
		proposalID := uuid.New()
		return &LLMResponse{
			Message: "I have optimized the draft content and created a proposal.",
			Proposals: []models.AIProposal{
				{
					ID:                proposalID,
					WorkspaceID:       snapshot.WorkspaceID,
					ProjectID:         snapshot.ProjectID,
					ContextSnapshotID: snapshot.ID,
					ProposalType:      "source_rewrite",
					TargetPlatform:    "wechat",
					Status:            "proposed",
					Summary:           "Scrubbed and polished content",
					Patch:             "@@ -1,3 +1,3 @@\n-Hello\n+Hello World Optimized",
					CreatedAt:         time.Now(),
				},
			},
		}, nil
	}
}

// RunSession executes the harness agentic loop, writing timeline events to PostgreSQL and streaming SSE responses
func (r *Runner) RunSession(ctx context.Context, sessionID uuid.UUID, createdByID uuid.UUID, userMessage string, sseChan chan<- string) error {
	// 1. Fetch drafting session
	var session models.AIDraftingSession
	if err := r.db.WithContext(ctx).First(&session, "id = ?", sessionID).Error; err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// 2. Log user message and timeline event
	userMsg := models.AIDraftingMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "user",
		Content:   userMessage,
		CreatedAt: time.Now(),
	}
	if err := r.db.WithContext(ctx).Create(&userMsg).Error; err != nil {
		return err
	}

	_, _ = r.eventMgr.LogEvent(ctx, sessionID, "message", userMsg)
	r.sendSSE(sseChan, "message", userMsg)

	adapter := &MockLLMAdapter{}

	for {
		// 3. Assemble and budget context snapshot
		snapshot, err := r.assembler.CreateSnapshot(ctx, session.ProjectID, createdByID, "drafting", AssembleOptions{
			ContextBudget: 50000,
		})
		if err != nil {
			_, _ = r.eventMgr.LogEvent(ctx, sessionID, "status", map[string]any{"status": "error", "error": err.Error()})
			r.sendSSE(sseChan, "error", map[string]any{"code": "context_budget_exceeded", "message": err.Error()})
			return err
		}

		// Update active snapshot on session
		r.db.WithContext(ctx).Model(&session).Update("active_context_snapshot_id", snapshot.ID)

		_, _ = r.eventMgr.LogEvent(ctx, sessionID, "status", map[string]any{"status": "context_assembled", "snapshot_id": snapshot.ID})
		r.sendSSE(sseChan, "status", "Context snapshot assembled successfully")

		// Retrieve all messages for this session to build history
		var messages []models.AIDraftingMessage
		if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages).Error; err != nil {
			return err
		}

		// 4. Query LLM
		r.sendSSE(sseChan, "status", "Querying LLM...")
		resp, err := adapter.Query(ctx, sessionID, messages, snapshot)
		if err != nil {
			_, _ = r.eventMgr.LogEvent(ctx, sessionID, "status", map[string]any{"status": "error", "error": err.Error()})
			return err
		}

		// Handle assistant message
		if resp.Message != "" {
			assistantMsg := models.AIDraftingMessage{
				ID:        uuid.New(),
				SessionID: sessionID,
				Role:      "assistant",
				Content:   resp.Message,
				CreatedAt: time.Now(),
			}
			r.db.WithContext(ctx).Create(&assistantMsg)
			_, _ = r.eventMgr.LogEvent(ctx, sessionID, "message", assistantMsg)
			r.sendSSE(sseChan, "message", assistantMsg)
		}

		// 5. Handle Tool Execution
		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				tc.CreatedAt = time.Now()
				r.db.WithContext(ctx).Create(&tc)
				_, _ = r.eventMgr.LogEvent(ctx, sessionID, "tool_call", tc)
				r.sendSSE(sseChan, "tool_call", tc)

				r.sendSSE(sseChan, "status", fmt.Sprintf("Running tool %s...", tc.ToolName))
				tool, exists := r.tools[tc.ToolName]
				var resultStr string
				var runErr error
				start := time.Now()

				if !exists {
					runErr = fmt.Errorf("tool %s not registered", tc.ToolName)
				} else {
					resultStr, runErr = tool.Execute(ctx, json.RawMessage(tc.Arguments))
				}
				duration := time.Since(start)

				if runErr != nil {
					tc.Error = runErr.Error()
				} else {
					tc.Result = resultStr
				}
				tc.DurationMs = int(duration.Milliseconds())
				r.db.WithContext(ctx).Save(&tc)

				_, _ = r.eventMgr.LogEvent(ctx, sessionID, "tool_result", tc)
				r.sendSSE(sseChan, "tool_result", tc)
			}
			// Let the agent inspect the tool results by running another turn
			continue
		}

		// 6. Handle Proposals
		if len(resp.Proposals) > 0 {
			for _, prop := range resp.Proposals {
				prop.SessionID = &sessionID
				r.db.WithContext(ctx).Create(&prop)
				_, _ = r.eventMgr.LogEvent(ctx, sessionID, "proposal", prop)
				r.sendSSE(sseChan, "proposal", prop)
			}
			break
		}

		break
	}

	_, _ = r.eventMgr.LogEvent(ctx, sessionID, "status", map[string]any{"status": "completed"})
	r.sendSSE(sseChan, "status", "Session run completed successfully")
	return nil
}

func (r *Runner) sendSSE(ch chan<- string, event string, data any) {
	if ch == nil {
		return
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	ch <- fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(bytes))
}
