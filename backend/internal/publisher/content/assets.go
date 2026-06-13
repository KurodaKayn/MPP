package content

import (
	"encoding/json"
	"strings"
)

type adaptedContentAsset struct {
	SourceURL string `json:"source_url"`
	Type      string `json:"type"`
}

type adaptedContentWithAssets struct {
	Assets []adaptedContentAsset `json:"assets"`
}

func ExtractImageAssetSources(raw []byte) []string {
	var adapted adaptedContentWithAssets
	if err := json.Unmarshal(raw, &adapted); err != nil {
		return nil
	}

	sources := make([]string, 0, len(adapted.Assets))
	seen := make(map[string]struct{}, len(adapted.Assets))
	for _, asset := range adapted.Assets {
		if !strings.EqualFold(strings.TrimSpace(asset.Type), "image") {
			continue
		}
		source := strings.TrimSpace(asset.SourceURL)
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		sources = append(sources, source)
	}
	return sources
}

func ExtractFirstImageAssetSource(raw []byte) string {
	sources := ExtractImageAssetSources(raw)
	if len(sources) == 0 {
		return ""
	}
	return sources[0]
}

func ExtractCoverImageURL(raw []byte) string {
	var config struct {
		CoverImageURL string `json:"cover_image_url"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return ""
	}
	return strings.TrimSpace(config.CoverImageURL)
}

func SelectCoverImageSource(rawConfig []byte, adaptedContent []byte) string {
	if source := ExtractCoverImageURL(rawConfig); source != "" {
		return source
	}
	return ExtractFirstImageAssetSource(adaptedContent)
}
