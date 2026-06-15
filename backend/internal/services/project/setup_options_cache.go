package project

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const contentSetupOptionsCachePrefix = "mpp:dashboard:content-setup:v1"
const contentSetupOptionsCacheVersion = 1
const contentSetupOptionsRefreshTimeout = 15 * time.Second
const contentSetupOptionsInvalidateTimeout = 2 * time.Second

const (
	contentSetupResourceTemplates     = "content-templates"
	contentSetupResourceBrandProfiles = "brand-profiles"
)

type contentSetupOptionsCachePayload[T any] struct {
	Version     int       `json:"version"`
	Resource    string    `json:"resource"`
	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Response    T         `json:"response"`
}

func (s *Service) getCachedContentTemplates(userID uuid.UUID, workspaceID uuid.UUID) (*dto.ContentTemplatesResponse, error) {
	return getCachedContentSetupOptions(s, contentSetupResourceTemplates, userID, workspaceID, func(svc *Service) (*dto.ContentTemplatesResponse, error) {
		return svc.computeContentTemplates(userID, workspaceID)
	}, func(resp dto.ContentTemplatesResponse) bool {
		return resp.Items != nil
	})
}

func (s *Service) getCachedBrandProfiles(userID uuid.UUID, workspaceID uuid.UUID) (*dto.BrandProfilesResponse, error) {
	return getCachedContentSetupOptions(s, contentSetupResourceBrandProfiles, userID, workspaceID, func(svc *Service) (*dto.BrandProfilesResponse, error) {
		return svc.computeBrandProfiles(workspaceID)
	}, func(resp dto.BrandProfilesResponse) bool {
		return resp.Items != nil
	})
}

func getCachedContentSetupOptions[T any](
	s *Service,
	resource string,
	userID uuid.UUID,
	workspaceID uuid.UUID,
	compute func(*Service) (*T, error),
	valid func(T) bool,
) (*T, error) {
	if !s.canUseContentSetupOptionsCache() {
		return compute(s)
	}

	ctx := s.requestContext()
	cacheKey := contentSetupOptionsCacheKey(resource, userID, workspaceID)
	if resp, hit, _ := cachedContentSetupOptions(ctx, s, cacheKey, resource, userID, workspaceID, valid); hit {
		return resp, nil
	}

	if s.cacheGroup == nil {
		return refreshContentSetupOptionsCache(ctx, s, cacheKey, resource, userID, workspaceID, compute)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := contentSetupOptionsRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if resp, hit, err := cachedContentSetupOptions(refreshCtx, refreshSvc, cacheKey, resource, userID, workspaceID, valid); hit {
			return resp, nil
		} else if err != nil {
			return compute(refreshSvc)
		}
		return refreshContentSetupOptionsCache(refreshCtx, refreshSvc, cacheKey, resource, userID, workspaceID, compute)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*T); ok {
			return resp, nil
		}
		return compute(s)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func cachedContentSetupOptions[T any](
	ctx context.Context,
	s *Service,
	cacheKey string,
	resource string,
	userID uuid.UUID,
	workspaceID uuid.UUID,
	valid func(T) bool,
) (*T, bool, error) {
	cached, err := s.cache.Get(ctx, cacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if resp, ok := decodeContentSetupOptionsPayload(cached, resource, userID, workspaceID, valid); ok {
		return resp, true, nil
	}
	return nil, false, nil
}

func decodeContentSetupOptionsPayload[T any](
	cached []byte,
	resource string,
	userID uuid.UUID,
	workspaceID uuid.UUID,
	valid func(T) bool,
) (*T, bool) {
	var payload contentSetupOptionsCachePayload[T]
	if err := json.Unmarshal(cached, &payload); err != nil {
		return nil, false
	}
	if !contentSetupOptionsPayloadValid(payload, resource, userID, workspaceID, valid) {
		return nil, false
	}
	return &payload.Response, true
}

func refreshContentSetupOptionsCache[T any](
	ctx context.Context,
	s *Service,
	cacheKey string,
	resource string,
	userID uuid.UUID,
	workspaceID uuid.UUID,
	compute func(*Service) (*T, error),
) (*T, error) {
	resp, err := compute(s)
	if err != nil {
		return nil, err
	}
	payload := contentSetupOptionsCachePayload[T]{
		Version:     contentSetupOptionsCacheVersion,
		Resource:    resource,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Response:    *resp,
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = s.cache.Set(ctx, cacheKey, encoded, s.cacheTTL).Err()
	}
	return resp, nil
}

func (s *Service) invalidateContentTemplateOptionsCache(userID uuid.UUID, workspaceID uuid.UUID, scope string) {
	if s.cache == nil {
		return
	}
	ctx, cancel := contentSetupOptionsInvalidationContext(s.requestContext())
	defer cancel()
	if scope == models.ContentTemplateScopeWorkspace {
		deleteContentSetupOptionsCacheKeys(ctx, s.cache, contentSetupOptionsWorkspacePattern(contentSetupResourceTemplates, workspaceID))
		return
	}
	deleteContentSetupOptionsCacheKeys(ctx, s.cache, contentSetupOptionsUserPattern(contentSetupResourceTemplates, userID))
}

func (s *Service) invalidateBrandProfileOptionsCache(workspaceID uuid.UUID) {
	if s.cache == nil {
		return
	}
	ctx, cancel := contentSetupOptionsInvalidationContext(s.requestContext())
	defer cancel()
	deleteContentSetupOptionsCacheKeys(ctx, s.cache, contentSetupOptionsWorkspacePattern(contentSetupResourceBrandProfiles, workspaceID))
}

func deleteContentSetupOptionsCacheKeys(ctx context.Context, client *redis.Client, pattern string) {
	var cursor uint64
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

func (s *Service) canUseContentSetupOptionsCache() bool {
	if s.cache == nil || s.cacheTTL <= 0 {
		return false
	}
	stickyUntil, sticky := dbrouter.StickyWriterUntil(s.requestContext())
	return !sticky || !stickyUntil.After(time.Now())
}

func contentSetupOptionsCacheKey(resource string, userID uuid.UUID, workspaceID uuid.UUID) string {
	return contentSetupOptionsCachePrefix + ":" + resource + ":user:" + userID.String() + ":workspace:" + workspaceID.String()
}

func contentSetupOptionsUserPattern(resource string, userID uuid.UUID) string {
	return contentSetupOptionsCachePrefix + ":" + resource + ":user:" + userID.String() + ":workspace:*"
}

func contentSetupOptionsWorkspacePattern(resource string, workspaceID uuid.UUID) string {
	return contentSetupOptionsCachePrefix + ":" + resource + ":user:*:workspace:" + workspaceID.String()
}

func contentSetupOptionsRefreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), contentSetupOptionsRefreshTimeout)
}

func contentSetupOptionsInvalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), contentSetupOptionsInvalidateTimeout)
}

func contentSetupOptionsPayloadValid[T any](
	payload contentSetupOptionsCachePayload[T],
	resource string,
	userID uuid.UUID,
	workspaceID uuid.UUID,
	valid func(T) bool,
) bool {
	if payload.Version != contentSetupOptionsCacheVersion {
		return false
	}
	if payload.Resource != resource || payload.UserID != userID || payload.WorkspaceID != workspaceID {
		return false
	}
	return valid(payload.Response)
}
