package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownloadAndProcessSupportsBase64DataURL(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "false")

	data, err := DownloadAndProcess("data:image/png;base64,aGVsbG8=")

	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}
