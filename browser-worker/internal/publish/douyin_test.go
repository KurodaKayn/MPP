package publish

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCoverImageWritesDecodedContentAndCleansUp(t *testing.T) {
	path, cleanup, err := writeCoverImage("SGVsbG8gRG91eWlu", "cover.PNG")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.True(t, strings.HasSuffix(path, ".png"))
	data, err := os.ReadFile(path) // #nosec G304 -- path is created by writeCoverImage in this test.
	require.NoError(t, err)
	assert.Equal(t, "Hello Douyin", string(data))

	cleanup()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestWriteCoverImageRejectsMissingOrInvalidData(t *testing.T) {
	for _, tc := range []struct {
		name    string
		encoded string
		wantErr string
	}{
		{name: "blank", encoded: " ", wantErr: "requires a cover image"},
		{name: "invalid base64", encoded: "not base64", wantErr: "failed to decode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, cleanup, err := writeCoverImage(tc.encoded, "cover.png")

			require.Error(t, err)
			assert.Empty(t, path)
			assert.Nil(t, cleanup)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCoverImageExtAllowsOnlyBrowserUploadImageTypes(t *testing.T) {
	assert.Equal(t, ".jpg", coverImageExt("cover.svg"))
	assert.Equal(t, ".jpg", coverImageExt("cover"))
	assert.Equal(t, ".jpeg", coverImageExt("cover.JPEG"))
	assert.Equal(t, ".webp", coverImageExt(" cover.webp "))
}

func TestJsonStringEscapesDouyinContentForEvaluationScript(t *testing.T) {
	raw := "first line\n</script><script>alert(1)</script>"

	encoded := jsonString(raw)

	var decoded string
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	assert.Equal(t, raw, decoded)
	assert.NotContains(t, encoded, "</script>")
}
