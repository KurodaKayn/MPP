package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

// EventLogManager manages serialization and replay of events in an AI session
type EventLogManager struct {
	db   *gorm.DB
	mu   sync.Mutex
	last time.Time
}

func NewEventLogManager(db *gorm.DB) *EventLogManager {
	return &EventLogManager{db: db}
}

// LogEvent serializes and records a new session event in the database
func (m *EventLogManager) LogEvent(ctx context.Context, sessionID uuid.UUID, eventType string, payload any) (*models.AISessionEvent, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}

	m.mu.Lock()
	createdAt := time.Now().UTC()
	if !m.last.IsZero() && !createdAt.After(m.last) {
		createdAt = m.last.Add(time.Microsecond)
	}
	m.last = createdAt
	m.mu.Unlock()

	event := models.AISessionEvent{
		ID:        uuid.New(),
		SessionID: sessionID,
		EventType: eventType,
		Payload:   datatypes.JSON(payloadBytes),
		CreatedAt: createdAt,
	}

	if err := m.db.WithContext(ctx).Create(&event).Error; err != nil {
		return nil, fmt.Errorf("failed to save session event: %w", err)
	}

	return &event, nil
}

// GetEvents retrieves all events for a given session sorted chronologically
func (m *EventLogManager) GetEvents(ctx context.Context, sessionID uuid.UUID) ([]models.AISessionEvent, error) {
	var events []models.AISessionEvent
	err := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at asc, id asc").
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session events: %w", err)
	}
	return events, nil
}

// ReplayTimeline reconstructs the timeline of events to rebuild a UI state
type TimelineItem struct {
	ID        uuid.UUID      `json:"id"`
	SessionID uuid.UUID      `json:"session_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type ReplayResult struct {
	SessionID     uuid.UUID                  `json:"session_id"`
	Events        []TimelineItem             `json:"events"`
	ModelMessages []models.AIDraftingMessage `json:"model_messages"`
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
			ID:        e.ID,
			SessionID: e.SessionID,
			EventType: e.EventType,
			Payload:   payloadMap,
			CreatedAt: e.CreatedAt,
		}
	}
	return timeline, nil
}

func (m *EventLogManager) ReplaySession(ctx context.Context, sessionID uuid.UUID) (*ReplayResult, error) {
	timeline, err := m.Replay(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var messages []models.AIDraftingMessage
	if err := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at asc, id asc").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch session messages: %w", err)
	}

	return &ReplayResult{
		SessionID:     sessionID,
		Events:        timeline,
		ModelMessages: messages,
	}, nil
}
