package platformaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const xOAuth2StateKeyPrefix = "mpp:x_oauth2_state:"

type XOAuth2StateStore interface {
	Store(_ context.Context, state string, pending xOAuth2PendingState, ttl time.Duration) error
	Consume(_ context.Context, state string) (xOAuth2PendingState, bool, error)
}

type MemoryXOAuth2StateStore struct {
	mu     sync.Mutex
	states map[string]xOAuth2PendingState
}

func NewMemoryXOAuth2StateStore() *MemoryXOAuth2StateStore {
	return &MemoryXOAuth2StateStore{states: make(map[string]xOAuth2PendingState)}
}

func (s *MemoryXOAuth2StateStore) Store(ctx context.Context, state string, pending xOAuth2PendingState, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for existingState, existingPending := range s.states {
		if now.After(existingPending.ExpiresAt) {
			delete(s.states, existingState)
		}
	}
	if pending.ExpiresAt.IsZero() {
		pending.ExpiresAt = now.Add(ttl)
	}
	s.states[state] = pending
	return nil
}

func (s *MemoryXOAuth2StateStore) Consume(ctx context.Context, state string) (xOAuth2PendingState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, ok := s.states[state]
	if ok {
		delete(s.states, state)
	}
	return pending, ok, nil
}

type RedisXOAuth2StateStore struct {
	client redis.UniversalClient
	prefix string
}

type redisXOAuth2PendingState struct {
	UserID       string    `json:"user_id"`
	WorkspaceID  string    `json:"workspace_id,omitempty"`
	CodeVerifier string    `json:"code_verifier"`
	RedirectURI  string    `json:"redirect_uri"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewRedisXOAuth2StateStore(client redis.UniversalClient) *RedisXOAuth2StateStore {
	return &RedisXOAuth2StateStore{
		client: client,
		prefix: xOAuth2StateKeyPrefix,
	}
}

func (s *RedisXOAuth2StateStore) Store(ctx context.Context, state string, pending xOAuth2PendingState, ttl time.Duration) error {
	payload, err := json.Marshal(redisXOAuth2PendingStateFromPending(pending))
	if err != nil {
		return err
	}

	stored, err := s.client.SetNX(ctx, s.key(state), payload, ttl).Result()
	if err != nil {
		return err
	}
	if !stored {
		return fmt.Errorf("x oauth2 state collision")
	}
	return nil
}

func (s *RedisXOAuth2StateStore) Consume(ctx context.Context, state string) (xOAuth2PendingState, bool, error) {
	raw, err := s.client.GetDel(ctx, s.key(state)).Bytes()
	if errors.Is(err, redis.Nil) {
		return xOAuth2PendingState{}, false, nil
	}
	if err != nil {
		return xOAuth2PendingState{}, false, err
	}

	var payload redisXOAuth2PendingState
	if err := json.Unmarshal(raw, &payload); err != nil {
		return xOAuth2PendingState{}, false, err
	}
	pending, err := payload.toPending()
	if err != nil {
		return xOAuth2PendingState{}, false, err
	}
	return pending, true, nil
}

func (s *RedisXOAuth2StateStore) key(state string) string {
	return s.prefix + state
}

func redisXOAuth2PendingStateFromPending(pending xOAuth2PendingState) redisXOAuth2PendingState {
	payload := redisXOAuth2PendingState{
		UserID:       pending.UserID.String(),
		CodeVerifier: pending.CodeVerifier,
		RedirectURI:  pending.RedirectURI,
		ExpiresAt:    pending.ExpiresAt,
	}
	if pending.WorkspaceID != uuid.Nil {
		payload.WorkspaceID = pending.WorkspaceID.String()
	}
	return payload
}

func (p redisXOAuth2PendingState) toPending() (xOAuth2PendingState, error) {
	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return xOAuth2PendingState{}, fmt.Errorf("invalid x oauth2 user_id: %w", err)
	}
	var workspaceID uuid.UUID
	if p.WorkspaceID != "" {
		workspaceID, err = uuid.Parse(p.WorkspaceID)
		if err != nil {
			return xOAuth2PendingState{}, fmt.Errorf("invalid x oauth2 workspace_id: %w", err)
		}
	}
	return xOAuth2PendingState{
		UserID:       userID,
		WorkspaceID:  workspaceID,
		CodeVerifier: p.CodeVerifier,
		RedirectURI:  p.RedirectURI,
		ExpiresAt:    p.ExpiresAt,
	}, nil
}
