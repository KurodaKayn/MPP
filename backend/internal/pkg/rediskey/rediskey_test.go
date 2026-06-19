package rediskey

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartNormalizesUnsafeCharacters(t *testing.T) {
	require.Equal(t, "tenant:workspace-1", Part(" Tenant:Workspace 1 "))
	require.Equal(t, Unknown, Part(" @@@ "))
}

func TestTagBuildsRedisHashTag(t *testing.T) {
	require.Equal(t, "{session:11111111-1111-4111-8111-111111111111}", Tag("session", "11111111-1111-4111-8111-111111111111"))
	require.Equal(t, "{tenant:unknown}", Tag("tenant", ""))
}

func TestShareTagRequiresMatchingHashTags(t *testing.T) {
	require.True(t, ShareTag(
		"mpp:browser:stream-current:{session:abc}",
		"mpp:browser:stream-token:{session:abc}:hash",
	))
	require.False(t, ShareTag(
		"mpp:browser:stream-current:{session:abc}",
		"mpp:browser:stream-token:{session:def}:hash",
	))
	require.False(t, ShareTag(
		"mpp:browser:stream-current:abc",
		"mpp:browser:stream-token:{session:abc}:hash",
	))
}
