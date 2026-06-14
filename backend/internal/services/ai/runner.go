package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	EventStatus     = "status"
	EventMessage    = "message"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventProposal   = "proposal"
	EventError      = "error"

	defaultDraftingContextBudget = 50000
	defaultMaxHarnessTurns       = 6
	defaultMaxToolResultSize     = 24000
)

var ErrHarnessTurnLimitExceeded = errors.New("ai harness turn limit exceeded")
var ErrToolMissing = errors.New("tool missing")
var ErrToolSchemaInvalid = errors.New("tool schema invalid")
var ErrToolPermissionDenied = errors.New("tool permission denied")

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	IsReadOnly() bool
	IsConcurrencySafe() bool
	RequiresPermission() bool
	MaxResultSize() int
	MapResult(result string, runErr error) (string, bool)
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

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
func (t *MockTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true}
}
func (t *MockTool) IsReadOnly() bool         { return true }
func (t *MockTool) IsConcurrencySafe() bool  { return true }
func (t *MockTool) RequiresPermission() bool { return false }
func (t *MockTool) MaxResultSize() int       { return defaultMaxToolResultSize }
func (t *MockTool) MapResult(result string, runErr error) (string, bool) {
	if runErr != nil {
		return runErr.Error(), true
	}
	return limitToolResult(result, t.MaxResultSize()), false
}
func (t *MockTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.fn(ctx, args)
}

type LLMAdapter interface {
	Query(ctx context.Context, sessionID uuid.UUID, messages []ModelVisibleMessage, snapshot *models.AIContextSnapshot, toolResults []models.AIToolCall) (*LLMResponse, error)
}

type Runner struct {
	db        *gorm.DB
	eventMgr  *EventLogManager
	assembler *AIContextAssembler
	quotaSvc  *QuotaService
	adapter   LLMAdapter
	tools     map[string]Tool
	maxTurns  int
}

func NewRunner(db *gorm.DB, eventMgr *EventLogManager, assembler *AIContextAssembler, quotaSvc *QuotaService) *Runner {
	return NewRunnerWithAdapter(db, eventMgr, assembler, quotaSvc, nil)
}

func NewRunnerWithAdapter(db *gorm.DB, eventMgr *EventLogManager, assembler *AIContextAssembler, quotaSvc *QuotaService, adapter LLMAdapter) *Runner {
	if adapter == nil {
		adapter = &MockLLMAdapter{}
	}
	return &Runner{
		db:        db,
		eventMgr:  eventMgr,
		assembler: assembler,
		quotaSvc:  quotaSvc,
		adapter:   adapter,
		tools:     make(map[string]Tool),
		maxTurns:  defaultMaxHarnessTurns,
	}
}

func (r *Runner) RegisterTool(t Tool) {
	if t == nil || strings.TrimSpace(t.Name()) == "" {
		return
	}
	r.tools[t.Name()] = t
}

func (r *Runner) SetMaxTurns(maxTurns int) {
	if maxTurns > 0 {
		r.maxTurns = maxTurns
	}
}

type LLMResponse struct {
	Message   string              `json:"message,omitempty"`
	ToolCalls []models.AIToolCall `json:"tool_calls,omitempty"`
	Proposals []models.AIProposal `json:"proposals,omitempty"`
	Usage     *dto.AIUsage        `json:"usage,omitempty"`
}

type MockLLMAdapter struct {
	Turn int
}

func (m *MockLLMAdapter) Query(_ context.Context, sessionID uuid.UUID, _ []ModelVisibleMessage, snapshot *models.AIContextSnapshot, toolResults []models.AIToolCall) (*LLMResponse, error) {
	m.Turn++
	if len(toolResults) == 0 {
		toolCallID := uuid.New()
		return &LLMResponse{
			Message: "I will read the current project context first.",
			ToolCalls: []models.AIToolCall{
				{
					ID:        toolCallID,
					SessionID: sessionID,
					ToolName:  "read_project_context",
					Version:   "1.0",
					Arguments: datatypes.JSON(fmt.Appendf(nil, `{"project_id":%q}`, snapshot.ProjectID.String())),
				},
			},
		}, nil
	}

	proposalID := uuid.New()
	return &LLMResponse{
		Message: "I created a draft proposal from the available project context.",
		Proposals: []models.AIProposal{
			{
				ID:                proposalID,
				WorkspaceID:       snapshot.WorkspaceID,
				ProjectID:         snapshot.ProjectID,
				ContextSnapshotID: snapshot.ID,
				ProposalType:      "source_rewrite",
				TargetPlatform:    "wechat",
				Status:            "proposed",
				Summary:           "Drafting harness skeleton proposal",
				Patch:             "@@ -1 +1 @@\n-" + firstLine(snapshot.SourceContent) + "\n+" + firstLine(snapshot.SourceContent) + "\n",
				CreatedAt:         time.Now(),
			},
		},
	}, nil
}

func (r *Runner) RunSession(ctx context.Context, sessionID uuid.UUID, createdByID uuid.UUID, userMessage string, sseChan chan<- string) error {
	var session models.AIDraftingSession
	if err := r.db.WithContext(ctx).First(&session, "id = ?", sessionID).Error; err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	tools := r.toolsForRun(session.ProjectID, createdByID)

	userMsg := models.AIDraftingMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      "user",
		Content:   strings.TrimSpace(userMessage),
		CreatedAt: time.Now(),
	}
	if userMsg.Content == "" {
		return r.fail(ctx, sessionID, sseChan, "invalid_request", "message is required", nil)
	}
	if err := r.db.WithContext(ctx).Create(&userMsg).Error; err != nil {
		return err
	}
	r.emit(ctx, sessionID, sseChan, EventMessage, userMsg, WithEventModelVisible(true))

	var toolResults []models.AIToolCall
	for turn := 1; turn <= r.maxTurns; turn++ {
		turnID := uuid.New()
		if err := ctx.Err(); err != nil {
			return r.fail(ctx, sessionID, sseChan, classifyHarnessError(err), err.Error(), err, WithEventTurnID(turnID))
		}
		if r.quotaSvc != nil {
			if err := r.quotaSvc.CheckQuota(ctx, session.WorkspaceID); err != nil {
				return r.fail(ctx, sessionID, sseChan, "quota_exceeded", err.Error(), err, WithEventTurnID(turnID))
			}
		}

		snapshot, err := r.assembler.CreateSnapshot(ctx, session.ProjectID, createdByID, "drafting", AssembleOptions{
			ContextBudget: defaultDraftingContextBudget,
		})
		if err != nil {
			return r.fail(ctx, sessionID, sseChan, "context_unavailable", err.Error(), err, WithEventTurnID(turnID))
		}
		if err := r.db.WithContext(ctx).Model(&session).Updates(map[string]any{
			"active_context_snapshot_id": snapshot.ID,
			"updated_at":                 time.Now(),
		}).Error; err != nil {
			return err
		}
		r.emit(ctx, sessionID, sseChan, EventStatus, map[string]any{
			"status":      "context_assembled",
			"snapshot_id": snapshot.ID,
			"turn":        turn,
		}, WithEventTurnID(turnID))

		replay, err := r.eventMgr.ReplaySession(ctx, sessionID)
		if err != nil {
			return err
		}
		messages := replay.ModelMessages

		r.emit(ctx, sessionID, sseChan, EventStatus, map[string]any{"status": "model_querying", "turn": turn}, WithEventTurnID(turnID))
		resp, err := r.adapter.Query(ctx, sessionID, messages, snapshot, toolResults)
		if err != nil {
			code := classifyHarnessError(err)
			if code == "tool_error" {
				code = "model_error"
			}
			return r.fail(ctx, sessionID, sseChan, code, err.Error(), err, WithEventTurnID(turnID))
		}

		if resp.Usage != nil && r.quotaSvc != nil {
			if err := r.quotaSvc.RecordUsage(ctx, session.WorkspaceID, createdByID, &sessionID, "drafting", resp.Usage); err != nil {
				log.Printf("[quota] RecordUsage failed workspace=%s session=%s kind=drafting: %v", session.WorkspaceID, sessionID, err)
			}
		}

		if strings.TrimSpace(resp.Message) != "" {
			assistantMsg := models.AIDraftingMessage{
				ID:        uuid.New(),
				SessionID: sessionID,
				Role:      "assistant",
				Content:   resp.Message,
				CreatedAt: time.Now(),
			}
			if err := r.db.WithContext(ctx).Create(&assistantMsg).Error; err != nil {
				return err
			}
			r.emit(ctx, sessionID, sseChan, EventMessage, assistantMsg, WithEventTurnID(turnID), WithEventModelVisible(true))
		}

		if len(resp.ToolCalls) > 0 {
			toolResults = toolResults[:0]
			for _, tc := range resp.ToolCalls {
				tc.SessionID = sessionID
				if tc.ID == uuid.Nil {
					tc.ID = uuid.New()
				}
				if strings.TrimSpace(tc.ToolUseID) == "" {
					tc.ToolUseID = "toolu_" + uuid.NewString()
				}
				if tc.Version == "" {
					tc.Version = "1.0"
				}
				tc.CreatedAt = time.Now()
				if len(tc.Arguments) > 0 && !json.Valid(tc.Arguments) {
					err := fmt.Errorf("%w: invalid arguments for tool %s", ErrToolSchemaInvalid, tc.ToolName)
					tc.Arguments = datatypes.JSON(`{}`)
					tc.Error = err.Error()
					if err := r.db.WithContext(ctx).Create(&tc).Error; err != nil {
						return err
					}
					r.emit(ctx, sessionID, sseChan, EventToolCall, tc, WithEventTurnID(turnID), WithEventToolUseID(tc.ToolUseID), WithEventModelVisible(true))
					r.emit(ctx, sessionID, sseChan, EventToolResult, tc, WithEventTurnID(turnID), WithEventToolUseID(tc.ToolUseID), WithEventModelVisible(true))
					return r.fail(ctx, sessionID, sseChan, classifyHarnessError(err), err.Error(), err, WithEventTurnID(turnID), WithEventToolUseID(tc.ToolUseID))
				}
				if err := r.db.WithContext(ctx).Create(&tc).Error; err != nil {
					return err
				}
				r.emit(ctx, sessionID, sseChan, EventToolCall, tc, WithEventTurnID(turnID), WithEventToolUseID(tc.ToolUseID), WithEventModelVisible(true))

				executed, err := r.executeTool(ctx, tools, tc)
				if err != nil {
					r.emit(ctx, sessionID, sseChan, EventToolResult, executed, WithEventTurnID(turnID), WithEventToolUseID(executed.ToolUseID), WithEventModelVisible(true))
					return r.fail(ctx, sessionID, sseChan, classifyHarnessError(err), err.Error(), err, WithEventTurnID(turnID), WithEventToolUseID(executed.ToolUseID))
				}
				toolResults = append(toolResults, executed)
				r.emit(ctx, sessionID, sseChan, EventToolResult, executed, WithEventTurnID(turnID), WithEventToolUseID(executed.ToolUseID), WithEventModelVisible(true))
			}
			continue
		}

		if len(resp.Proposals) > 0 {
			for _, prop := range resp.Proposals {
				prop.SessionID = &sessionID
				if prop.ID == uuid.Nil {
					prop.ID = uuid.New()
				}
				if prop.CreatedAt.IsZero() {
					prop.CreatedAt = time.Now()
				}
				if err := r.db.WithContext(ctx).Create(&prop).Error; err != nil {
					return err
				}
				r.emit(ctx, sessionID, sseChan, EventProposal, prop, WithEventTurnID(turnID))
			}
		}

		r.emit(ctx, sessionID, sseChan, EventStatus, map[string]any{"status": "completed"}, WithEventTurnID(turnID))
		return nil
	}

	return r.fail(ctx, sessionID, sseChan, "turn_limit_exceeded", ErrHarnessTurnLimitExceeded.Error(), ErrHarnessTurnLimitExceeded)
}

func (r *Runner) toolsForRun(projectID uuid.UUID, createdByID uuid.UUID) map[string]Tool {
	tools := make(map[string]Tool, len(r.tools)+1)
	maps.Copy(tools, r.tools)
	if _, exists := tools["read_project_context"]; !exists && r.assembler != nil {
		tools["read_project_context"] = NewReadProjectContextToolForProject(r.assembler, projectID, createdByID)
	}
	return tools
}

func (r *Runner) executeTool(ctx context.Context, tools map[string]Tool, tc models.AIToolCall) (models.AIToolCall, error) {
	tool, exists := tools[tc.ToolName]
	start := time.Now()
	var result string
	var runErr error
	if !exists {
		runErr = fmt.Errorf("%w: %s", ErrToolMissing, tc.ToolName)
	} else if len(tc.Arguments) > 0 && !json.Valid(tc.Arguments) {
		runErr = fmt.Errorf("%w: invalid arguments for tool %s", ErrToolSchemaInvalid, tc.ToolName)
	} else {
		result, runErr = tool.Execute(ctx, json.RawMessage(tc.Arguments))
	}
	tc.DurationMs = int(time.Since(start).Milliseconds())
	if exists {
		mapped, isError := tool.MapResult(result, runErr)
		if isError {
			tc.Error = mapped
		} else {
			tc.Result = mapped
		}
	} else if runErr != nil {
		tc.Error = runErr.Error()
	}
	if err := r.db.WithContext(ctx).Save(&tc).Error; err != nil {
		return tc, err
	}
	if runErr != nil {
		return tc, runErr
	}
	return tc, nil
}

func (r *Runner) fail(ctx context.Context, sessionID uuid.UUID, ch chan<- string, code string, message string, err error, opts ...EventLogOption) error {
	payload := map[string]any{"code": code, "message": message}
	eventCtx := context.WithoutCancel(ctx)
	r.emit(eventCtx, sessionID, ch, EventError, payload, opts...)
	if err != nil {
		return err
	}
	return errors.New(message)
}

func classifyHarnessError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "user_interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrToolPermissionDenied):
		return "permission_denied"
	case errors.Is(err, ErrToolSchemaInvalid):
		return "schema_error"
	case errors.Is(err, ErrToolMissing):
		return "missing_tool"
	default:
		return "tool_error"
	}
}

func (r *Runner) emit(ctx context.Context, sessionID uuid.UUID, ch chan<- string, eventType string, payload any, opts ...EventLogOption) {
	event, err := r.eventMgr.LogEvent(ctx, sessionID, eventType, payload, opts...)
	if err != nil {
		log.Printf("[ai] failed to append session event session=%s event=%s: %v", sessionID, eventType, err)
		return
	}
	writeSSE(ch, event.EventType, event.Payload)
}

func writeSSE(ch chan<- string, event string, data []byte) {
	if ch == nil {
		return
	}
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	ch <- fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(data))
}

func firstLine(s string) string {
	line := strings.TrimSpace(s)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	if line == "" {
		return "Draft"
	}
	return line
}

func limitToolResult(result string, maxSize int) string {
	if maxSize <= 0 || len(result) <= maxSize {
		return result
	}
	const marker = "\n... [TRUNCATED TOOL RESULT] ...\n"
	available := maxSize - len(marker)
	if available <= 0 {
		return result[:maxSize]
	}
	head := available / 2
	tail := available - head
	return result[:head] + marker + result[len(result)-tail:]
}
