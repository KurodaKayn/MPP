//go:build contentpipeline_integration

package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const tinyPNGDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="

func TestContentPipelineIntegrationProcessesDataURL(t *testing.T) {
	data, err := DownloadAndProcessForPlatform(tinyPNGDataURL, "wechat", "cover")

	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.GreaterOrEqual(t, len(data), 8)
}
