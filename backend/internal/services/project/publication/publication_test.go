package publication

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func TestDetailFromModelNormalizesEmptyContent(t *testing.T) {
	detail := DetailFromModel(models.ProjectPlatformPublication{}, true)

	require.NotNil(t, detail.AdaptedContent)
	require.Empty(t, detail.AdaptedContent)
}

func TestResponseDetailFromModelPreservesEmptyIncludedContent(t *testing.T) {
	detail := ResponseDetailFromModel(models.ProjectPlatformPublication{}, true)

	require.Nil(t, detail.AdaptedContent)
}

func TestResponseDetailFromModelSummarizesExcludedContent(t *testing.T) {
	pub := models.ProjectPlatformPublication{
		AdaptedContent: datatypes.JSON([]byte(`{"summary":"short","body":"hidden"}`)),
	}

	detail := ResponseDetailFromModel(pub, false)

	require.Equal(t, map[string]any{"summary": "short"}, detail.AdaptedContent)
}
