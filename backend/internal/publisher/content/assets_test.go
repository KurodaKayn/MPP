package content

import (
	"testing"

	"gorm.io/datatypes"
)

func TestExtractImageAssetSourcesUsesCompiledAssetDescriptors(t *testing.T) {
	raw := datatypes.JSON(`{
		"format": "html",
		"assets": [
			{"type":"image","source_url":" mpp://media/11111111-1111-4111-8111-111111111111 "},
			{"type":"image","source_url":"https://example.com/hero.png"},
			{"type":"image","source_url":"https://example.com/hero.png"},
			{"type":"video","source_url":"https://example.com/video.mp4"},
			{"type":"image","source_url":""}
		]
	}`)

	sources := ExtractImageAssetSources(raw)

	if len(sources) != 2 {
		t.Fatalf("expected 2 image sources, got %d: %#v", len(sources), sources)
	}
	if sources[0] != "mpp://media/11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected first source: %q", sources[0])
	}
	if sources[1] != "https://example.com/hero.png" {
		t.Fatalf("unexpected second source: %q", sources[1])
	}
}

func TestExtractFirstImageAssetSourceReturnsFirstCompiledImage(t *testing.T) {
	source := ExtractFirstImageAssetSource(datatypes.JSON(`{"assets":[{"type":"image","source_url":"https://example.com/first.png"},{"type":"image","source_url":"https://example.com/second.png"}]}`))

	if source != "https://example.com/first.png" {
		t.Fatalf("expected first image source, got %q", source)
	}
}

func TestSelectCoverImageSourcePrefersExplicitCoverImageURL(t *testing.T) {
	source := SelectCoverImageSource(
		datatypes.JSON(`{"cover_image_url":" https://example.com/cover.jpg "}`),
		datatypes.JSON(`{"assets":[{"type":"image","source_url":"https://example.com/body.png"}]}`),
	)

	if source != "https://example.com/cover.jpg" {
		t.Fatalf("expected explicit cover image source, got %q", source)
	}
}

func TestSelectCoverImageSourceFallsBackToFirstCompiledImage(t *testing.T) {
	source := SelectCoverImageSource(
		datatypes.JSON(`{}`),
		datatypes.JSON(`{"assets":[{"type":"image","source_url":"https://example.com/body.png"}]}`),
	)

	if source != "https://example.com/body.png" {
		t.Fatalf("expected compiled image fallback, got %q", source)
	}
}
