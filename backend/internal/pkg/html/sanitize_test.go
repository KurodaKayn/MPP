package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeStoredHTMLStripsActiveContent(t *testing.T) {
	sanitized := SanitizeStoredHTML(`
		<p onclick="alert(1)">Hello <strong>safe</strong></p>
		<script>alert(1)</script>
		<img src="javascript:alert(1)" onerror="alert(1)" alt="cover">
		<a href="java&#x0A;script:alert(1)">bad link</a>
		<svg onload="alert(1)"><circle></circle></svg>
	`)
	lower := strings.ToLower(sanitized)

	require.Contains(t, sanitized, "<p>Hello <strong>safe</strong></p>")
	require.Contains(t, sanitized, `<img alt="cover"/>`)
	require.Contains(t, sanitized, ">bad link</a>")
	require.NotContains(t, lower, "script")
	require.NotContains(t, lower, "onclick")
	require.NotContains(t, lower, "onerror")
	require.NotContains(t, lower, "javascript:")
	require.NotContains(t, lower, "<svg")
}

func TestSanitizeStoredHTMLKeepsSafeMediaAndLinks(t *testing.T) {
	sanitized := SanitizeStoredHTML(`
		<p><a href="https://example.com/post">safe</a></p>
		<img src="mpp://media/asset-1" data-mpp-media-id="asset-1" alt="asset">
		<img src="data:image/png;base64,aGVsbG8=" alt="inline">
	`)

	require.Contains(t, sanitized, `href="https://example.com/post"`)
	require.Contains(t, sanitized, `src="mpp://media/asset-1"`)
	require.Contains(t, sanitized, `data-mpp-media-id="asset-1"`)
	require.Contains(t, sanitized, `src="data:image/png;base64,aGVsbG8="`)
}
