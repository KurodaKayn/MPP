package platformaccount

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisXOAuth2StateStoreUsesMinimalPayload(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	store := NewRedisXOAuth2StateStore(client)

	pending := xOAuth2PendingState{
		UserID:       uuid.New(),
		WorkspaceID:  uuid.New(),
		CodeVerifier: "code-verifier",
		RedirectURI:  "https://app.example.com/callback",
		ExpiresAt:    time.Now().Add(10 * time.Minute).UTC(),
	}
	require.NoError(t, store.Store(context.Background(), "state-value", pending, time.Minute))

	keys := redisServer.Keys()
	require.Len(t, keys, 1)
	raw, err := client.Get(context.Background(), keys[0]).Bytes()
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, []string{"code_verifier", "expires_at", "redirect_uri", "user_id", "workspace_id"}, sortedMapKeys(payload))
	require.NotContains(t, payload, "client_id")
	require.NotContains(t, payload, "status")
	require.NotContains(t, payload, "redirect_url")

	consumed, ok, err := store.Consume(context.Background(), "state-value")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, pending.UserID, consumed.UserID)
	require.Equal(t, pending.WorkspaceID, consumed.WorkspaceID)
	require.Equal(t, pending.CodeVerifier, consumed.CodeVerifier)
	require.Equal(t, pending.RedirectURI, consumed.RedirectURI)
}

func TestRedisXOAuth2StateStoreRejectsInvalidWorkspaceID(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	store := NewRedisXOAuth2StateStore(client)

	raw, err := json.Marshal(redisXOAuth2PendingState{
		UserID:       uuid.NewString(),
		WorkspaceID:  "not-a-uuid",
		CodeVerifier: "code-verifier",
		RedirectURI:  "https://app.example.com/callback",
		ExpiresAt:    time.Now().Add(10 * time.Minute).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, client.Set(context.Background(), store.key("state-value"), raw, time.Minute).Err())

	_, ok, err := store.Consume(context.Background(), "state-value")
	require.Error(t, err)
	require.False(t, ok)
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
