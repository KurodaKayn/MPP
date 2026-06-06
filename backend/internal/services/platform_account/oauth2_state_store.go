package platformaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

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
	client *redis.Client
	prefix string
}

func NewRedisXOAuth2StateStore(client *redis.Client) *RedisXOAuth2StateStore {
	return &RedisXOAuth2StateStore{
		client: client,
		prefix: xOAuth2StateKeyPrefix,
	}
}

func (s *RedisXOAuth2StateStore) Store(ctx context.Context, state string, pending xOAuth2PendingState, ttl time.Duration) error {
	payload, err := json.Marshal(pending)
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

	var pending xOAuth2PendingState
	if err := json.Unmarshal(raw, &pending); err != nil {
		return xOAuth2PendingState{}, false, err
	}
	return pending, true, nil
}

func (s *RedisXOAuth2StateStore) key(state string) string {
	return s.prefix + state
}
