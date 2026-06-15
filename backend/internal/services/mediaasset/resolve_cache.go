package mediaasset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	resolvedMediaAssetCachePrefix     = "mpp:dashboard:media-assets:resolve:v1"
	resolvedMediaAssetCacheTTL        = 15 * time.Second
	resolvedMediaAssetInvalidateDelay = 2 * time.Second
)

var errResolvedMediaAssetCacheStale = errors.New("resolved media asset cache stale")

type resolvedMediaAssetCachePayload struct {
	AssetID     string    `json:"asset_id"`
	UserID      string    `json:"user_id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	ProjectID   string    `json:"project_id,omitempty"`
	Bucket      string    `json:"bucket"`
	ObjectKey   string    `json:"object_key"`
	Status      string    `json:"status"`
	URL         string    `json:"url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Service) cachedResolvedMediaAsset(assetID uuid.UUID, userID uuid.UUID) (dto.ResolvedMediaAsset, bool, error) {
	if !s.canUseResolvedMediaAssetCache() || assetID == uuid.Nil || userID == uuid.Nil {
		return dto.ResolvedMediaAsset{}, false, nil
	}

	ctx := s.requestContext()
	cacheKey := resolvedMediaAssetCacheKey(assetID, userID)
	cached, err := s.cache.Get(ctx, cacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return dto.ResolvedMediaAsset{}, false, nil
		}
		return dto.ResolvedMediaAsset{}, false, nil
	}

	payload, ok := decodeResolvedMediaAssetCachePayload(cached, assetID, userID, time.Now().UTC())
	if !ok {
		return dto.ResolvedMediaAsset{}, false, nil
	}
	if err := s.authorizeCachedResolvedMediaAsset(payload, userID); err != nil {
		if errors.Is(err, errResolvedMediaAssetCacheStale) {
			return dto.ResolvedMediaAsset{}, false, nil
		}
		return dto.ResolvedMediaAsset{}, true, err
	}
	return dto.ResolvedMediaAsset{
		AssetID:   assetID,
		URL:       payload.URL,
		ExpiresAt: payload.ExpiresAt,
	}, true, nil
}

func (s *Service) cacheResolvedMediaAsset(asset models.MediaAsset, userID uuid.UUID, item dto.ResolvedMediaAsset) {
	ttl := s.currentResolvedMediaAssetCacheTTL()
	if ttl <= 0 || asset.ID == uuid.Nil || userID == uuid.Nil {
		return
	}
	expiresIn := time.Until(item.ExpiresAt)
	if expiresIn <= 0 {
		return
	}
	if ttl >= expiresIn {
		ttl = expiresIn - time.Second
		if ttl <= 0 {
			ttl = expiresIn / 2
		}
	}
	if ttl <= 0 {
		return
	}

	payload := resolvedMediaAssetCachePayload{
		AssetID:   asset.ID.String(),
		UserID:    userID.String(),
		Bucket:    asset.Bucket,
		ObjectKey: asset.ObjectKey,
		Status:    asset.Status,
		URL:       item.URL,
		ExpiresAt: item.ExpiresAt,
	}
	if asset.WorkspaceID != nil && *asset.WorkspaceID != uuid.Nil {
		payload.WorkspaceID = asset.WorkspaceID.String()
	}
	if asset.ProjectID != nil && *asset.ProjectID != uuid.Nil {
		payload.ProjectID = asset.ProjectID.String()
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = s.cache.Set(s.requestContext(), resolvedMediaAssetCacheKey(asset.ID, userID), encoded, ttl).Err()
}

func (s *Service) authorizeCachedResolvedMediaAsset(payload resolvedMediaAssetCachePayload, userID uuid.UUID) error {
	if strings.TrimSpace(payload.ProjectID) != "" {
		projectID, err := uuid.Parse(payload.ProjectID)
		if err != nil || projectID == uuid.Nil {
			return ErrInvalidMediaAsset
		}
		var project models.Project
		if err := s.strongReadDB().
			Select("id", "user_id", "workspace_id").
			First(&project, "id = ?", projectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return s.authorizeCachedResolvedMediaAssetThroughCurrentAsset(payload, userID)
			}
			return err
		}
		_, err = s.projects.ProjectAccessRole(project, userID)
		return err
	}

	workspaceID, err := uuid.Parse(payload.WorkspaceID)
	if err != nil || workspaceID == uuid.Nil {
		return ErrInvalidMediaAsset
	}
	_, err = s.projects.WorkspaceProjectRole(workspaceID, userID)
	return err
}

func (s *Service) authorizeCachedResolvedMediaAssetThroughCurrentAsset(payload resolvedMediaAssetCachePayload, userID uuid.UUID) error {
	assetID, err := uuid.Parse(payload.AssetID)
	if err != nil || assetID == uuid.Nil {
		return ErrInvalidMediaAsset
	}
	asset, err := s.mediaAssetForRead(assetID, userID)
	if err != nil {
		return err
	}
	if asset.Status != models.MediaAssetStatusReady {
		return ErrMediaAssetNotReady
	}
	if asset.Bucket != payload.Bucket || asset.ObjectKey != payload.ObjectKey {
		return errResolvedMediaAssetCacheStale
	}
	return nil
}

func decodeResolvedMediaAssetCachePayload(cached []byte, assetID uuid.UUID, userID uuid.UUID, now time.Time) (resolvedMediaAssetCachePayload, bool) {
	var payload resolvedMediaAssetCachePayload
	if err := json.Unmarshal(cached, &payload); err != nil {
		return resolvedMediaAssetCachePayload{}, false
	}
	if payload.AssetID != assetID.String() || payload.UserID != userID.String() {
		return resolvedMediaAssetCachePayload{}, false
	}
	if payload.Status != models.MediaAssetStatusReady {
		return resolvedMediaAssetCachePayload{}, false
	}
	if strings.TrimSpace(payload.URL) == "" ||
		strings.TrimSpace(payload.Bucket) == "" ||
		strings.TrimSpace(payload.ObjectKey) == "" ||
		!payload.ExpiresAt.After(now) {
		return resolvedMediaAssetCachePayload{}, false
	}
	if strings.TrimSpace(payload.ProjectID) == "" && strings.TrimSpace(payload.WorkspaceID) == "" {
		return resolvedMediaAssetCachePayload{}, false
	}
	return payload, true
}

func (s *Service) invalidateResolvedMediaAssetCache(assetID uuid.UUID) {
	if s == nil || s.cache == nil || assetID == uuid.Nil {
		return
	}
	ctx, cancel := resolvedMediaAssetInvalidationContext(s.requestContext())
	defer cancel()
	deleteResolvedMediaAssetCacheKeys(ctx, s.cache, assetID)
}

func deleteResolvedMediaAssetCacheKeys(ctx context.Context, client *redis.Client, assetID uuid.UUID) {
	var cursor uint64
	pattern := resolvedMediaAssetCachePrefix + ":" + assetID.String() + ":*"
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
		if next == 0 {
			return
		}
		cursor = next
	}
}

func resolvedMediaAssetInvalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), resolvedMediaAssetInvalidateDelay)
}

func (s *Service) canUseResolvedMediaAssetCache() bool {
	return s.currentResolvedMediaAssetCacheTTL() > 0
}

func (s *Service) currentResolvedMediaAssetCacheTTL() time.Duration {
	if s == nil || s.cache == nil || s.cacheTTL <= 0 || s.storageConfig.DownloadURLTTL <= 0 {
		return 0
	}
	ttl := s.cacheTTL
	if ttl >= s.storageConfig.DownloadURLTTL {
		ttl = s.storageConfig.DownloadURLTTL - time.Second
	}
	if ttl <= 0 {
		ttl = s.storageConfig.DownloadURLTTL / 2
	}
	return ttl
}

func resolvedMediaAssetCacheKey(assetID uuid.UUID, userID uuid.UUID) string {
	return fmt.Sprintf("%s:%s:actor:%s", resolvedMediaAssetCachePrefix, assetID, userID)
}
