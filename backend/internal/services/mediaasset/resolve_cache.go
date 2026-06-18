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

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
)

const (
	resolvedMediaAssetCachePrefix     = "mpp:dashboard:media-assets:resolve:v1"
	resolvedMediaAssetCacheTTL        = 15 * time.Second
	resolvedMediaAssetInvalidateDelay = 2 * time.Second
)

var errResolvedMediaAssetCacheStale = errors.New("resolved media asset cache stale")

type resolvedMediaAssetCachePayload struct {
	Bucket    string    `json:"bucket"`
	ObjectKey string    `json:"object_key"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) cachedResolvedMediaAsset(assetID uuid.UUID, userID uuid.UUID) (dto.ResolvedMediaAsset, bool, error) {
	if !s.canUseResolvedMediaAssetCache() || assetID == uuid.Nil || userID == uuid.Nil {
		return dto.ResolvedMediaAsset{}, false, nil
	}

	ctx := s.requestContext()
	cacheKey := resolvedMediaAssetCacheKey(assetID, userID)
	cached, err := redisdegrade.Call(s.cacheGuard, func() ([]byte, error) {
		return s.cache.Get(ctx, cacheKey).Bytes()
	})
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return dto.ResolvedMediaAsset{}, false, nil
		}
		return dto.ResolvedMediaAsset{}, false, nil
	}

	payload, ok := decodeResolvedMediaAssetCachePayload(cached, time.Now().UTC())
	if !ok {
		return dto.ResolvedMediaAsset{}, false, nil
	}
	if err := s.authorizeCachedResolvedMediaAsset(assetID, payload, userID); err != nil {
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
		Bucket:    asset.Bucket,
		ObjectKey: asset.ObjectKey,
		URL:       item.URL,
		ExpiresAt: item.ExpiresAt,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = redisdegrade.Do(s.cacheGuard, func() error {
		return s.cache.Set(s.requestContext(), resolvedMediaAssetCacheKey(asset.ID, userID), encoded, ttl).Err()
	})
}

func (s *Service) authorizeCachedResolvedMediaAsset(assetID uuid.UUID, payload resolvedMediaAssetCachePayload, userID uuid.UUID) error {
	if assetID == uuid.Nil {
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

func decodeResolvedMediaAssetCachePayload(cached []byte, now time.Time) (resolvedMediaAssetCachePayload, bool) {
	var payload resolvedMediaAssetCachePayload
	if err := json.Unmarshal(cached, &payload); err != nil {
		return resolvedMediaAssetCachePayload{}, false
	}
	if strings.TrimSpace(payload.URL) == "" ||
		strings.TrimSpace(payload.Bucket) == "" ||
		strings.TrimSpace(payload.ObjectKey) == "" ||
		!payload.ExpiresAt.After(now) {
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
	deleteResolvedMediaAssetCacheKeys(ctx, s.cache, s.cacheGuard, assetID)
}

func deleteResolvedMediaAssetCacheKeys(ctx context.Context, client *redis.Client, guard *redisdegrade.Guard, assetID uuid.UUID) {
	var cursor uint64
	pattern := resolvedMediaAssetCachePrefix + ":" + assetID.String() + ":*"
	for {
		type scanResult struct {
			keys []string
			next uint64
		}
		result, err := redisdegrade.Call(guard, func() (scanResult, error) {
			keys, next, err := client.Scan(ctx, cursor, pattern, 100).Result()
			return scanResult{keys: keys, next: next}, err
		})
		if err != nil {
			return
		}
		if len(result.keys) > 0 {
			_ = redisdegrade.Do(guard, func() error {
				return client.Del(ctx, result.keys...).Err()
			})
		}
		if result.next == 0 {
			return
		}
		cursor = result.next
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
