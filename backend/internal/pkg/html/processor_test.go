package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessHTMLImageSourcesOnlyRewritesMatchingImageSrc(t *testing.T) {
	source := "https://example.com/image.png?width=100&height=200"
	processed, err := ProcessHTMLImageSources(
		`<p>https://example.com/image.png?width=100&amp;height=200</p><img src="https://example.com/image.png?width=100&amp;height=200" alt="match"><img src="https://example.com/other.png" alt="other">`,
		[]string{source},
		func(url string) (string, error) {
			require.Equal(t, source, url)
			return "processed-object", nil
		},
		func(objectRef string) (string, error) {
			require.Equal(t, "processed-object", objectRef)
			return "https://mmbiz.qpic.cn/uploaded.jpg", nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(processed, "https://mmbiz.qpic.cn/uploaded.jpg"))
	require.Contains(t, processed, `src="https://mmbiz.qpic.cn/uploaded.jpg"`)
	require.Contains(t, processed, `src="https://example.com/other.png"`)
	require.Contains(t, processed, `>https://example.com/image.png?width=100&amp;height=200</p>`)
}
