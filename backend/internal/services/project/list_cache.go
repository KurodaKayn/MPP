package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/pkg/cachettl"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
)

const dashboardProjectListCachePrefix = "mpp:dashboard:projects:list:v2"
const dashboardProjectListCacheGenerationKey = "mpp:dashboard:projects:list-generation:v2:{dashboard:projects-list}"
const dashboardProjectListDegradedGeneration = "degraded"
const dashboardProjectListRefreshTimeout = 15 * time.Second
const dashboardProjectListInvalidateTimeout = 2 * time.Second

type dashboardProjectListCacheParams struct {
	Generation   string `json:"generation"`
	Cursor       string `json:"cursor,omitempty"`
	Page         int    `json:"page"`
	Limit        int    `json:"limit"`
	Status       string `json:"status,omitempty"`
	FilterUserID string `json:"filter_user_id,omitempty"`
	Platform     string `json:"platform,omitempty"`
	ScopeUserID  string `json:"scope_user_id,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	ActorUserID  string `json:"actor_user_id,omitempty"`
}

type dashboardProjectListCachePayload struct {
	Items      []dto.ProjectListItem `json:"items"`
	Cursor     string                `json:"cursor,omitempty"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	Total      int64                 `json:"total"`
	TotalPages int                   `json:"total_pages"`
}

type dashboardProjectListCacheCompute func(*Service) (*dto.PaginationResponse, error)

func (s *Service) getCachedDashboardProjectList(cursor string, page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	params := dashboardProjectListCacheParams{
		Cursor:       cursor,
		Page:         page,
		Limit:        limit,
		Status:       status,
		FilterUserID: filterUserID,
		Platform:     platform,
		ScopeUserID:  uuidStringValue(scopeUserID),
	}
	return s.getCachedProjectList(params, func(svc *Service) (*dto.PaginationResponse, error) {
		return svc.computeProjectList(cursor, page, limit, status, filterUserID, platform, scopeUserID)
	})
}

func (s *Service) getCachedWorkspaceProjectList(workspaceID uuid.UUID, actorUserID uuid.UUID, cursor string, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	params := dashboardProjectListCacheParams{
		Cursor:      cursor,
		Page:        page,
		Limit:       limit,
		Status:      status,
		Platform:    platform,
		WorkspaceID: workspaceID.String(),
		ActorUserID: actorUserID.String(),
	}
	return s.getCachedProjectList(params, func(svc *Service) (*dto.PaginationResponse, error) {
		return svc.computeWorkspaceProjectList(workspaceID, actorUserID, cursor, page, limit, status, platform)
	})
}

func (s *Service) getCachedProjectList(params dashboardProjectListCacheParams, compute dashboardProjectListCacheCompute) (*dto.PaginationResponse, error) {
	ctx := s.requestContext()
	generation, err := s.dashboardProjectListCacheGeneration(ctx)
	if err != nil {
		params.Generation = dashboardProjectListDegradedGeneration
		return s.computeProjectListSingleflight(ctx, dashboardProjectListCacheKey(params), compute)
	}
	params.Generation = generation
	cacheKey := dashboardProjectListCacheKey(params)
	if resp, hit, err := s.cachedDashboardProjectList(ctx, cacheKey, params.Cursor, params.Page, params.Limit); hit {
		return resp, nil
	} else if err != nil {
		return compute(s)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardProjectListRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if resp, hit, err := refreshSvc.cachedDashboardProjectList(refreshCtx, cacheKey, params.Cursor, params.Page, params.Limit); hit {
			return resp, nil
		} else if err != nil {
			return compute(refreshSvc)
		}

		return refreshSvc.refreshDashboardProjectListCache(refreshCtx, cacheKey, compute)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*dto.PaginationResponse); ok {
			return resp, nil
		}
		return compute(s)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) computeProjectListSingleflight(ctx context.Context, cacheKey string, compute dashboardProjectListCacheCompute) (*dto.PaginationResponse, error) {
	if s.cacheGroup == nil {
		return compute(s)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardProjectListRefreshContext(ctx)
		defer cancel()
		return compute(s.WithContext(refreshCtx))
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*dto.PaginationResponse); ok {
			return resp, nil
		}
		return compute(s)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) cachedDashboardProjectList(ctx context.Context, cacheKey string, cursor string, page, limit int) (*dto.PaginationResponse, bool, error) {
	cached, err := redisdegrade.CallWork(s.projectListGuard, "cache_read", func() ([]byte, error) {
		return s.cache.Get(ctx, cacheKey).Bytes()
	})
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if resp, ok := decodeDashboardProjectListPayload(cached, cursor, page, limit); ok {
		return resp, true, nil
	}
	return nil, false, nil
}

func decodeDashboardProjectListPayload(cached []byte, cursor string, page, limit int) (*dto.PaginationResponse, bool) {
	var payload dashboardProjectListCachePayload
	if err := json.Unmarshal(cached, &payload); err != nil {
		return nil, false
	}
	if !dashboardProjectListPayloadValid(payload, cursor, page, limit) {
		return nil, false
	}
	return dashboardProjectListPayloadToResponse(payload), true
}

func (s *Service) refreshDashboardProjectListCache(ctx context.Context, cacheKey string, compute dashboardProjectListCacheCompute) (*dto.PaginationResponse, error) {
	resp, err := compute(s)
	if err != nil {
		return nil, err
	}
	payload, ok := dashboardProjectListPayloadFromResponse(resp)
	if !ok {
		return resp, nil
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = redisdegrade.DoWork(s.projectListGuard, "cache_write", func() error {
			return s.cache.Set(ctx, cacheKey, encoded, cachettl.Jitter(s.cacheTTL, cacheKey)).Err()
		})
	}
	return resp, nil
}

func (s *Service) canUseDashboardProjectListCache() bool {
	if s.cache == nil || s.cacheTTL <= 0 {
		return false
	}
	stickyUntil, sticky := dbrouter.StickyWriterUntil(s.requestContext())
	return !sticky || !stickyUntil.After(time.Now())
}

func (s *Service) InvalidateDashboardProjectListCache(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx != nil {
		s = s.WithContext(ctx)
	}
	s.invalidateDashboardProjectListCache()
}

func (s *Service) invalidateDashboardProjectListCache() {
	if s.cache == nil {
		return
	}
	ctx, cancel := dashboardProjectListInvalidationContext(s.requestContext())
	defer cancel()
	_ = redisdegrade.DoWork(s.projectListGuard, "cache_invalidate", func() error {
		return s.cache.Incr(ctx, dashboardProjectListCacheGenerationKey).Err()
	})
	deleteDashboardProjectListCacheKeys(ctx, s.cache, s.projectListGuard)
}

func (s *Service) invalidateDashboardCaches(includeStats bool) {
	ctx := s.requestContext()
	s.InvalidateDashboardProjectListCache(ctx)
	if includeStats && s.statsCache != nil {
		s.statsCache.InvalidateDashboardStatsCache(ctx)
	}
}

func (s *Service) invalidateDashboardScopedStatsCache() {
	if s.statsCache == nil {
		return
	}
	s.statsCache.InvalidateDashboardScopedStatsCache(s.requestContext())
}

func (s *Service) refreshProjectReadModel(projectID uuid.UUID) {
	if s.readModels == nil || projectID == uuid.Nil {
		return
	}
	s.readModels.RefreshProjectAsync(s.requestContext(), projectID)
}

func deleteDashboardProjectListCacheKeys(ctx context.Context, client *redis.Client, guard *redisdegrade.Guard) {
	var cursor uint64
	for {
		type scanResult struct {
			keys []string
			next uint64
		}
		result, err := redisdegrade.CallWork(guard, "cache_invalidate", func() (scanResult, error) {
			keys, next, err := client.Scan(ctx, cursor, dashboardProjectListCachePrefix+":*", 100).Result()
			return scanResult{keys: keys, next: next}, err
		})
		if err != nil {
			return
		}
		if len(result.keys) > 0 {
			for _, key := range result.keys {
				_ = redisdegrade.DoWork(guard, "cache_invalidate", func() error {
					return client.Del(ctx, key).Err()
				})
			}
		}
		if result.next == 0 {
			return
		}
		cursor = result.next
	}
}

func dashboardProjectListRefreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardProjectListRefreshTimeout)
}

func dashboardProjectListInvalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardProjectListInvalidateTimeout)
}

func (s *Service) dashboardProjectListCacheGeneration(ctx context.Context) (string, error) {
	generation, err := redisdegrade.CallWork(s.projectListGuard, "cache_read", func() (string, error) {
		return s.cache.Get(ctx, dashboardProjectListCacheGenerationKey).Result()
	})
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return generation, err
}

func dashboardProjectListCacheKey(params dashboardProjectListCacheParams) string {
	encoded, err := json.Marshal(params)
	if err != nil {
		return fmt.Sprintf("%s:%d:%d", dashboardProjectListCachePrefix, params.Page, params.Limit)
	}
	sum := sha256.Sum256(encoded)
	return dashboardProjectListCachePrefix + ":" + rediskey.Tag("dashboard", "projects-list") + ":" + hex.EncodeToString(sum[:])
}

func uuidStringValue(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func dashboardProjectListPayloadFromResponse(resp *dto.PaginationResponse) (dashboardProjectListCachePayload, bool) {
	items, ok := resp.Items.([]dto.ProjectListItem)
	if !ok {
		return dashboardProjectListCachePayload{}, false
	}
	return dashboardProjectListCachePayload{
		Items:      items,
		Cursor:     resp.Cursor,
		NextCursor: resp.NextCursor,
		HasMore:    resp.HasMore,
		Page:       resp.Page,
		Limit:      resp.Limit,
		Total:      resp.Total,
		TotalPages: resp.TotalPages,
	}, true
}

func dashboardProjectListPayloadValid(payload dashboardProjectListCachePayload, cursor string, page, limit int) bool {
	if payload.Items == nil {
		return false
	}
	if payload.Cursor != cursor {
		return false
	}
	if payload.Page != page || payload.Limit != limit {
		return false
	}
	if payload.Total < 0 || payload.TotalPages < 0 {
		return false
	}
	return true
}

func dashboardProjectListPayloadToResponse(payload dashboardProjectListCachePayload) *dto.PaginationResponse {
	return &dto.PaginationResponse{
		Items:      payload.Items,
		Cursor:     payload.Cursor,
		NextCursor: payload.NextCursor,
		HasMore:    payload.HasMore,
		Page:       payload.Page,
		Limit:      payload.Limit,
		Total:      payload.Total,
		TotalPages: payload.TotalPages,
	}
}
