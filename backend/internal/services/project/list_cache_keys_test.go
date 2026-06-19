package project

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
)

func TestDashboardProjectListKeysUseFamilyHashTag(t *testing.T) {
	cacheKey := dashboardProjectListCacheKey(dashboardProjectListCacheParams{
		Generation: "1",
		Page:       1,
		Limit:      20,
	})

	require.True(t, rediskey.ShareTag(cacheKey, dashboardProjectListCacheGenerationKey))
	tag, ok := rediskey.ExtractTag(cacheKey)
	require.True(t, ok)
	require.Equal(t, "dashboard:projects-list", tag)
}
