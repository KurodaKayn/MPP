package publication

import (
	"encoding/json"
	"strings"

	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	platformcapabilities "github.com/kurodakayn/mpp-backend/internal/platformcapabilities"
	"github.com/kurodakayn/mpp-backend/internal/services/project/contentsetup"
	"github.com/kurodakayn/mpp-backend/internal/services/project/projecterr"
	"github.com/kurodakayn/mpp-backend/internal/services/project/publicationselection"
	"github.com/kurodakayn/mpp-backend/internal/services/publicationpayload"
)

var allowedPlatforms = platformcapabilities.ProjectPlatformSet()

func NormalizePlatforms(input []string) ([]string, error) {
	seen := map[string]struct{}{}
	platforms := make([]string, 0, len(input))

	for _, raw := range input {
		platform := strings.TrimSpace(raw)
		if platform == "" {
			continue
		}
		if _, ok := allowedPlatforms[platform]; !ok {
			return nil, projecterr.ErrInvalidProject
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		platforms = append(platforms, platform)
	}

	return platforms, nil
}

func PendingConfigForTemplate(title, summary, coverImageURL string, template *models.ContentTemplate) publicationselection.ConfigForPlatform {
	return func(platform string) (datatypes.JSON, error) {
		config, err := publicationpayload.DefaultConfig(title, summary, coverImageURL)
		if err != nil {
			return nil, err
		}
		return contentsetup.MergePublicationConfig(config, contentsetup.ContentTemplatePlatformConfig(template, platform))
	}
}

func DefaultConfigForProjectTitle(title string) publicationselection.ConfigForPlatform {
	return func(string) (datatypes.JSON, error) {
		return publicationpayload.DefaultConfig(title, "", "")
	}
}

func DetailFromModel(pub models.ProjectPlatformPublication, includeContent bool) dto.PublicationDetail {
	return detailFromModel(pub, includeContent, true)
}

func ResponseDetailFromModel(pub models.ProjectPlatformPublication, includeContent bool) dto.PublicationDetail {
	return detailFromModel(pub, includeContent, false)
}

func detailFromModel(pub models.ProjectPlatformPublication, includeContent bool, normalizeEmptyContent bool) dto.PublicationDetail {
	var rawConfig map[string]any
	_ = json.Unmarshal(pub.Config, &rawConfig)
	safeConfig := FilterConfig(rawConfig)

	var rawContent map[string]any
	_ = json.Unmarshal(pub.AdaptedContent, &rawContent)
	safeContent := rawContent
	if !includeContent {
		safeContent = SummarizeAdaptedContent(rawContent)
	}
	if normalizeEmptyContent && safeContent == nil {
		safeContent = map[string]any{}
	}

	return dto.PublicationDetail{
		ID:             pub.ID,
		Platform:       pub.Platform,
		Enabled:        pub.Enabled,
		Status:         pub.Status,
		DraftStatus:    pub.DraftStatus,
		ReviewStatus:   pub.ReviewStatus,
		SyncRequired:   pub.SyncRequired,
		ErrorMessage:   pub.ErrorMessage,
		Config:         safeConfig,
		AdaptedContent: safeContent,
		PublishURL:     pub.PublishURL,
		RemoteID:       pub.RemoteID,
		RetryCount:     pub.RetryCount,
		LastAttemptAt:  pub.LastAttemptAt,
		PublishedAt:    pub.PublishedAt,
		CreatedAt:      pub.CreatedAt,
		UpdatedAt:      pub.UpdatedAt,
	}
}

func FilterConfig(raw map[string]any) map[string]any {
	safe := make(map[string]any)
	allowedKeys := []string{"title", "tags", "cover_image", "topics", "category", "original_declaration", "username"}
	for _, key := range allowedKeys {
		if val, ok := raw[key]; ok {
			safe[key] = val
		}
	}
	return safe
}

func SummarizeAdaptedContent(raw map[string]any) map[string]any {
	safe := make(map[string]any)
	if summary, ok := raw["summary"]; ok {
		safe["summary"] = summary
	} else {
		safe["summary"] = "Content adapted (no summary available)"
	}
	if format, ok := raw["format"]; ok {
		safe["format"] = format
	}
	return safe
}
