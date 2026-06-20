package publicationpayload

import (
	"encoding/json"
	"strings"

	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/textutil"
)

func BuildPending(title, summary, coverImageURL string) (datatypes.JSON, datatypes.JSON, string, error) {
	config, err := DefaultConfig(title, summary, coverImageURL)
	if err != nil {
		return nil, nil, "", err
	}

	return config, datatypes.JSON([]byte(`{}`)), models.PublicationStatusPending, nil
}

func DefaultConfig(title, summary, coverImageURL string) (datatypes.JSON, error) {
	digest := strings.TrimSpace(summary)
	if digest == "" {
		digest = title
	}
	config := map[string]any{
		"digest": textutil.TruncateRunes(digest, 120),
		"title":  title,
	}
	if coverImageURL := strings.TrimSpace(coverImageURL); coverImageURL != "" {
		config["cover_image_url"] = coverImageURL
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}
