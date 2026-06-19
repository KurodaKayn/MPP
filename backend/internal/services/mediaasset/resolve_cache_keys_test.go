package mediaasset

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
)

func TestResolvedMediaAssetCacheKeyUsesAssetHashTag(t *testing.T) {
	assetID := uuid.New()
	userID := uuid.New()

	key := resolvedMediaAssetCacheKey(assetID, userID)

	tag, ok := rediskey.ExtractTag(key)
	require.True(t, ok)
	require.Equal(t, "asset:"+assetID.String(), tag)
	require.Equal(t, resolvedMediaAssetCachePrefix+":"+rediskey.Tag("asset", assetID.String())+":actor:"+userID.String(), key)
}
