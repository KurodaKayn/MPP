package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

const publishMediaObjectRefPrefix = "mpp://media/"

var publishMediaObjectRefPattern = regexp.MustCompile(`mpp://media/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

var ErrPublishMediaStorageUnavailable = errors.New("publish media storage unavailable")
var ErrPublishMediaAssetNotReady = errors.New("publish media asset not ready")

func (s *Service) preparePublicationMediaRefs(ctx context.Context, project models.Project, pub models.ProjectPlatformPublication) (models.ProjectPlatformPublication, error) {
	refs := collectPublicationMediaRefs(pub.Config, pub.AdaptedContent)
	if len(refs) == 0 {
		return pub, nil
	}

	replacements := make(map[string]string, len(refs))
	for ref, assetID := range refs {
		url, err := s.presignPublishMediaAsset(ctx, project, assetID)
		if err != nil {
			return models.ProjectPlatformPublication{}, err
		}
		replacements[ref] = url
	}

	config, err := replaceMediaRefsInJSON(pub.Config, replacements)
	if err != nil {
		return models.ProjectPlatformPublication{}, fmt.Errorf("prepare publication config media refs: %w", err)
	}
	adaptedContent, err := replaceMediaRefsInJSON(pub.AdaptedContent, replacements)
	if err != nil {
		return models.ProjectPlatformPublication{}, fmt.Errorf("prepare publication adapted content media refs: %w", err)
	}

	pub.Config = config
	pub.AdaptedContent = adaptedContent
	return pub, nil
}

func (s *Service) presignPublishMediaAsset(ctx context.Context, project models.Project, assetID uuid.UUID) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.objectStorage == nil || !s.storageConfig.Enabled || strings.TrimSpace(s.storageConfig.Bucket) == "" || s.storageConfig.DownloadURLTTL <= 0 {
		return "", ErrPublishMediaStorageUnavailable
	}

	db := s.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}

	var asset models.MediaAsset
	if err := db.First(&asset, "id = ?", assetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("publish media asset not found: %s", assetID)
		}
		return "", err
	}
	if asset.ProjectID == nil || *asset.ProjectID != project.ID {
		return "", ErrForbidden
	}
	if asset.Status != models.MediaAssetStatusReady {
		return "", fmt.Errorf("%w: %s", ErrPublishMediaAssetNotReady, assetID)
	}
	if strings.TrimSpace(asset.Bucket) == "" || strings.TrimSpace(asset.ObjectKey) == "" {
		return "", fmt.Errorf("publish media asset has no object location: %s", assetID)
	}

	presigned, err := s.objectStorage.PresignGetObject(ctx, objectstorage.GetObjectInput{
		Bucket:  asset.Bucket,
		Key:     asset.ObjectKey,
		Expires: s.storageConfig.DownloadURLTTL,
	})
	if err != nil {
		return "", fmt.Errorf("presign publish media asset: %w", err)
	}
	return presigned.URL, nil
}

func collectPublicationMediaRefs(values ...datatypes.JSON) map[string]uuid.UUID {
	refs := map[string]uuid.UUID{}
	for _, value := range values {
		for _, match := range publishMediaObjectRefPattern.FindAllStringSubmatch(string(value), -1) {
			if len(match) != 2 || !strings.HasPrefix(match[0], publishMediaObjectRefPrefix) {
				continue
			}
			assetID, err := uuid.Parse(match[1])
			if err != nil {
				continue
			}
			refs[match[0]] = assetID
		}
	}
	return refs
}

func replaceMediaRefsInJSON(raw datatypes.JSON, replacements map[string]string) (datatypes.JSON, error) {
	if len(raw) == 0 || len(replacements) == 0 {
		return raw, nil
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return datatypes.JSON([]byte(replaceMediaRefsInString(string(raw), replacements))), nil
	}

	value = replaceMediaRefsInValue(value, replacements)
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(encoded), nil
}

func replaceMediaRefsInValue(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case string:
		return replaceMediaRefsInString(typed, replacements)
	case []any:
		for i, item := range typed {
			typed[i] = replaceMediaRefsInValue(item, replacements)
		}
		return typed
	case map[string]any:
		for key, item := range typed {
			typed[key] = replaceMediaRefsInValue(item, replacements)
		}
		return typed
	default:
		return value
	}
}

func replaceMediaRefsInString(value string, replacements map[string]string) string {
	for ref, replacement := range replacements {
		value = strings.ReplaceAll(value, ref, replacement)
	}
	return value
}
