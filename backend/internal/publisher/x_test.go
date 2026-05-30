package publisher

import (
	"strings"
	"testing"
)

func TestXWeightedLengthCountsCJKAndEmojiAsDouble(t *testing.T) {
	text := "abc\u4e2d\u6587\U0001F600"

	if got := xWeightedLength(text); got != 9 {
		t.Fatalf("expected weighted length 9, got %d", got)
	}
}

func TestBuildXPostTextTruncatesByWeightedLength(t *testing.T) {
	text := buildXPostText("", strings.Repeat("\u4e2d", 200), xCharacterLimit)

	if got := xWeightedLength(text); got > xCharacterLimit {
		t.Fatalf("expected weighted length <= %d, got %d", xCharacterLimit, got)
	}
	if !strings.HasSuffix(text, "...") {
		t.Fatalf("expected truncated text to end with ellipsis marker, got %q", text)
	}
}

func TestXWeightedLengthCountsURLsAsTransformedLength(t *testing.T) {
	text := "go https://example.com/really/long/path"

	if got := xWeightedLength(text); got != 26 {
		t.Fatalf("expected URL weighted length 26, got %d", got)
	}
}
