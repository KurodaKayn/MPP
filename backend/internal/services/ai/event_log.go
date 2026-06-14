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

// EventLogManager manages serialization and replay of events in an AI session
type EventLogManager struct {
	db *gorm.DB
}

func NewEventLogManager(db *gorm.DB) *EventLogManager {
	return &EventLogManager{db: db}
}

type EventLogOption func(*eventLogOptions)

type eventLogOptions struct {
	turnID       *uuid.UUID
	toolUseID    string
	modelVisible bool
}

func WithEventTurnID(turnID uuid.UUID) EventLogOption {
	return func(opts *eventLogOptions) {
		if turnID != uuid.Nil {
			opts.turnID = &turnID
		}
	}
}

func WithEventToolUseID(toolUseID string) EventLogOption {
	return func(opts *eventLogOptions) {
		opts.toolUseID = toolUseID
	}
}

func WithEventModelVisible(modelVisible bool) EventLogOption {
	return func(opts *eventLogOptions) {
		opts.modelVisible = modelVisible
	}
}

// LogEvent serializes and records a new session event in the database
func (m *EventLogManager) LogEvent(ctx context.Context, sessionID uuid.UUID, eventType string, payload any, optionFns ...EventLogOption) (*models.AISessionEvent, error) {
	var opts eventLogOptions
	for _, fn := range optionFns {
		if fn != nil {
			fn(&opts)
		}
	}

	var event *models.AISessionEvent
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lastSequence uint64
		if err := tx.Model(&models.AISessionEvent{}).
			Where("session_id = ?", sessionID).
			Select("COALESCE(MAX(sequence), 0)").
			Scan(&lastSequence).Error; err != nil {
			return fmt.Errorf("failed to allocate session event sequence: %w", err)
		}

		nextSequence := lastSequence + 1
		payloadMap, err := eventPayloadEnvelope(payload, nextSequence, opts)
		if err != nil {
			return err
		}
		payloadBytes, err := json.Marshal(payloadMap)
		if err != nil {
			return fmt.Errorf("failed to marshal event payload: %w", err)
		}

		event = &models.AISessionEvent{
			ID:           uuid.New(),
			SessionID:    sessionID,
			Sequence:     nextSequence,
			TurnID:       opts.turnID,
			ToolUseID:    opts.toolUseID,
			EventType:    eventType,
			ModelVisible: opts.modelVisible,
			Payload:      datatypes.JSON(payloadBytes),
			CreatedAt:    time.Now().UTC(),
		}

		if err := tx.Create(event).Error; err != nil {
			return fmt.Errorf("failed to save session event: %w", err)
		}
		return nil
	})

	return event, err
}

// GetEvents retrieves all events for a given session sorted chronologically
func (m *EventLogManager) GetEvents(ctx context.Context, sessionID uuid.UUID) ([]models.AISessionEvent, error) {
	var events []models.AISessionEvent
	err := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("sequence asc, created_at asc, id asc").
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session events: %w", err)
	}
	return events, nil
}

// ReplayTimeline reconstructs the timeline of events to rebuild a UI state
type TimelineItem struct {
	ID           uuid.UUID      `json:"id"`
	SessionID    uuid.UUID      `json:"session_id"`
	Sequence     uint64         `json:"sequence"`
	TurnID       *uuid.UUID     `json:"turn_id,omitempty"`
	ToolUseID    string         `json:"tool_use_id,omitempty"`
	EventType    string         `json:"event_type"`
	ModelVisible bool           `json:"model_visible"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
}

type ReplayResult struct {
	SessionID     uuid.UUID             `json:"session_id"`
	Events        []TimelineItem        `json:"events"`
	ModelMessages []ModelVisibleMessage `json:"model_messages"`
}

// Replay reconstructs the session timeline as a list of structured JSON timeline items
func (m *EventLogManager) Replay(ctx context.Context, sessionID uuid.UUID) ([]TimelineItem, error) {
	events, err := m.GetEvents(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	timeline := make([]TimelineItem, len(events))
	for i, e := range events {
		var payloadMap map[string]any
		if len(e.Payload) > 0 {
			if err := json.Unmarshal(e.Payload, &payloadMap); err != nil {
				payloadMap = map[string]any{"raw": string(e.Payload)}
			}
		}
		if payloadMap == nil {
			payloadMap = map[string]any{}
		}
		timeline[i] = TimelineItem{
			ID:           e.ID,
			SessionID:    e.SessionID,
			Sequence:     e.Sequence,
			TurnID:       e.TurnID,
			ToolUseID:    e.ToolUseID,
			EventType:    e.EventType,
			ModelVisible: e.ModelVisible,
			Payload:      payloadMap,
			CreatedAt:    e.CreatedAt,
		}
	}
	return timeline, nil
}

func (m *EventLogManager) ReplaySession(ctx context.Context, sessionID uuid.UUID) (*ReplayResult, error) {
	timeline, err := m.Replay(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return &ReplayResult{
		SessionID:     sessionID,
		Events:        timeline,
		ModelMessages: BuildModelVisibleMessages(timeline),
	}, nil
}

type ModelVisibleMessage struct {
	Role    string              `json:"role"`
	Content []ModelVisibleBlock `json:"content"`
}

type ModelVisibleBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

func BuildModelVisibleMessages(timeline []TimelineItem) []ModelVisibleMessage {
	messages := make([]ModelVisibleMessage, 0, len(timeline))
	for _, item := range timeline {
		if !item.ModelVisible {
			continue
		}
		switch item.EventType {
		case EventMessage:
			role := payloadString(item.Payload, "role", "Role")
			content := payloadString(item.Payload, "content", "Content")
			if role == "" || content == "" {
				continue
			}
			messages = append(messages, ModelVisibleMessage{
				Role:    role,
				Content: []ModelVisibleBlock{{Type: "text", Text: content}},
			})
		case EventToolCall:
			toolUseID := payloadString(item.Payload, "tool_use_id", "ToolUseID")
			if toolUseID == "" {
				toolUseID = item.ToolUseID
			}
			toolName := payloadString(item.Payload, "tool_name", "ToolName")
			if toolName == "" {
				continue
			}
			block := ModelVisibleBlock{
				Type:  "tool_use",
				ID:    toolUseID,
				Name:  toolName,
				Input: payloadMap(item.Payload, "arguments", "Arguments"),
			}
			if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
				messages[len(messages)-1].Content = append(messages[len(messages)-1].Content, block)
				continue
			}
			messages = append(messages, ModelVisibleMessage{
				Role:    "assistant",
				Content: []ModelVisibleBlock{block},
			})
		case EventToolResult:
			toolUseID := payloadString(item.Payload, "tool_use_id", "ToolUseID")
			if toolUseID == "" {
				toolUseID = item.ToolUseID
			}
			result := payloadString(item.Payload, "result", "Result")
			errorMessage := payloadString(item.Payload, "error", "Error")
			block := ModelVisibleBlock{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   result,
				IsError:   errorMessage != "",
			}
			if block.IsError {
				block.Content = errorMessage
			}
			messages = append(messages, ModelVisibleMessage{
				Role:    "user",
				Content: []ModelVisibleBlock{block},
			})
		case EventError:
			toolUseID := payloadString(item.Payload, "tool_use_id", "ToolUseID")
			if toolUseID == "" {
				toolUseID = item.ToolUseID
			}
			if toolUseID == "" {
				continue
			}
			messages = append(messages, ModelVisibleMessage{
				Role: "user",
				Content: []ModelVisibleBlock{{
					Type:      "tool_result",
					ToolUseID: toolUseID,
					Content:   payloadString(item.Payload, "message"),
					IsError:   true,
				}},
			})
		}
	}
	return ensureToolResultPairing(messages)
}

func ensureToolResultPairing(messages []ModelVisibleMessage) []ModelVisibleMessage {
	paired := make([]ModelVisibleMessage, 0, len(messages))
	openToolUses := make(map[string]bool)
	for _, message := range messages {
		if message.Role == "assistant" {
			for _, block := range message.Content {
				if block.Type == "tool_use" && block.ID != "" {
					openToolUses[block.ID] = true
				}
			}
			paired = append(paired, message)
			continue
		}
		if message.Role != "user" {
			paired = append(paired, message)
			continue
		}
		filtered := message.Content[:0]
		for _, block := range message.Content {
			if block.Type != "tool_result" {
				filtered = append(filtered, block)
				continue
			}
			if openToolUses[block.ToolUseID] {
				filtered = append(filtered, block)
				delete(openToolUses, block.ToolUseID)
			}
		}
		if len(filtered) > 0 {
			message.Content = filtered
			paired = append(paired, message)
		}
	}
	for toolUseID := range openToolUses {
		paired = append(paired, ModelVisibleMessage{
			Role: "user",
			Content: []ModelVisibleBlock{{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   "[Tool result missing due to internal error]",
				IsError:   true,
			}},
		})
	}
	return paired
}

func eventPayloadEnvelope(payload any, sequence uint64, opts eventLogOptions) (map[string]any, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil || payloadMap == nil {
		payloadMap = map[string]any{"data": string(payloadBytes)}
	}
	payloadMap["sequence"] = sequence
	payloadMap["model_visible"] = opts.modelVisible
	if opts.turnID != nil {
		payloadMap["turn_id"] = opts.turnID.String()
	}
	if opts.toolUseID != "" {
		payloadMap["tool_use_id"] = opts.toolUseID
	}
	return payloadMap, nil
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if str, ok := value.(string); ok {
				return str
			}
		}
	}
	return ""
}

func payloadMap(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if mapped, ok := value.(map[string]any); ok {
			return mapped
		}
		if str, ok := value.(string); ok {
			var mapped map[string]any
			if err := json.Unmarshal([]byte(str), &mapped); err == nil {
				return mapped
			}
		}
	}
	return nil
}
