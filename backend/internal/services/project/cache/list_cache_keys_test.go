package cache

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
)

func TestDashboardProjectListKeysUseFamilyHashTag(t *testing.T) {
	key := cacheKey(Params{
		Generation: "1",
		Page:       1,
		Limit:      20,
	})

	require.True(t, rediskey.ShareTag(key, generationKey))
	tag, ok := rediskey.ExtractTag(key)
	require.True(t, ok)
	require.Equal(t, "dashboard:projects-list", tag)
}

func TestDashboardProjectListScanPatternUsesFamilyHashTag(t *testing.T) {
	key := cacheKey(Params{
		Generation: "1",
		Page:       1,
		Limit:      20,
	})

	require.True(t, rediskey.ShareTag(key, pattern))
	require.Contains(t, pattern, hashTag)
}
