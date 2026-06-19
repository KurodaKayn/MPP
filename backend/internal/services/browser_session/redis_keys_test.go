package browsersession

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
)

func TestBrowserSessionRedisKeysUseApprovedHashTags(t *testing.T) {
	sessionID := uuid.New()
	userID := uuid.New()

	require.Equal(t, "session:"+sessionID.String(), mustRedisTag(t, browserSessionKey(sessionID)))
	require.Equal(t, "user:"+userID.String(), mustRedisTag(t, browserSessionActiveKey(userID, "Douyin")))
	require.True(t, rediskey.ShareTag(
		browserSessionKey(sessionID),
		browserSessionStreamCurrentKey(sessionID),
		browserSessionStreamTokenKey(sessionID, "TOKEN-HASH"),
	))
	require.Equal(t, browserSessionStreamTokenPrefix+rediskey.Tag("session", sessionID.String())+":", browserSessionStreamTokenKeyPrefixFor(sessionID))
}

func mustRedisTag(t *testing.T, key string) string {
	t.Helper()

	tag, ok := rediskey.ExtractTag(key)
	require.True(t, ok, key)
	return tag
}
