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
)

const dashboardProjectListCachePrefix = "mpp:dashboard:projects:list:v2"
const dashboardProjectListCacheGenerationKey = "mpp:dashboard:projects:list-generation:v2"
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

func (s *Service) getCachedDashboardProjectList(cursor string, page, limit int, status, filterUserID, platform string) (*dto.PaginationResponse, error) {
	ctx := s.requestContext()
	generation, err := s.dashboardProjectListCacheGeneration(ctx)
	if err != nil {
		return s.computeProjectList(cursor, page, limit, status, filterUserID, platform, nil)
	}
	cacheKey := dashboardProjectListCacheKey(generation, cursor, page, limit, status, filterUserID, platform)
	if resp, hit, err := s.cachedDashboardProjectList(ctx, cacheKey, cursor, page, limit); hit {
		return resp, nil
	} else if err != nil {
		return s.computeProjectList(cursor, page, limit, status, filterUserID, platform, nil)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardProjectListRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if resp, hit, err := refreshSvc.cachedDashboardProjectList(refreshCtx, cacheKey, cursor, page, limit); hit {
			return resp, nil
		} else if err != nil {
			return refreshSvc.computeProjectList(cursor, page, limit, status, filterUserID, platform, nil)
		}

		return refreshSvc.refreshDashboardProjectListCache(refreshCtx, cacheKey, cursor, page, limit, status, filterUserID, platform)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*dto.PaginationResponse); ok {
			return resp, nil
		}
		return s.computeProjectList(cursor, page, limit, status, filterUserID, platform, nil)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) cachedDashboardProjectList(ctx context.Context, cacheKey string, cursor string, page, limit int) (*dto.PaginationResponse, bool, error) {
	cached, err := s.cache.Get(ctx, cacheKey).Bytes()
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

func (s *Service) refreshDashboardProjectListCache(ctx context.Context, cacheKey string, cursor string, page, limit int, status, filterUserID, platform string) (*dto.PaginationResponse, error) {
	resp, err := s.computeProjectList(cursor, page, limit, status, filterUserID, platform, nil)
	if err != nil {
		return nil, err
	}
	payload, ok := dashboardProjectListPayloadFromResponse(resp)
	if !ok {
		return resp, nil
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = s.cache.Set(ctx, cacheKey, encoded, s.cacheTTL).Err()
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
	_ = s.cache.Incr(ctx, dashboardProjectListCacheGenerationKey).Err()
	deleteDashboardProjectListCacheKeys(ctx, s.cache)
}

func (s *Service) invalidateDashboardCaches(includeStats bool) {
	ctx := s.requestContext()
	s.InvalidateDashboardProjectListCache(ctx)
	if includeStats && s.statsCache != nil {
		s.statsCache.InvalidateDashboardStatsCache(ctx)
	}
}

func (s *Service) refreshProjectReadModel(projectID uuid.UUID) {
	if s.readModels == nil || projectID == uuid.Nil {
		return
	}
	s.readModels.RefreshProjectAsync(s.requestContext(), projectID)
}

func deleteDashboardProjectListCacheKeys(ctx context.Context, client *redis.Client) {
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, dashboardProjectListCachePrefix+":*", 100).Result()
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

func dashboardProjectListRefreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardProjectListRefreshTimeout)
}

func dashboardProjectListInvalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardProjectListInvalidateTimeout)
}

func (s *Service) dashboardProjectListCacheGeneration(ctx context.Context) (string, error) {
	generation, err := s.cache.Get(ctx, dashboardProjectListCacheGenerationKey).Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return generation, err
}

func dashboardProjectListCacheKey(generation string, cursor string, page, limit int, status, filterUserID, platform string) string {
	params := dashboardProjectListCacheParams{
		Generation:   generation,
		Cursor:       cursor,
		Page:         page,
		Limit:        limit,
		Status:       status,
		FilterUserID: filterUserID,
		Platform:     platform,
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return fmt.Sprintf("%s:%d:%d", dashboardProjectListCachePrefix, page, limit)
	}
	sum := sha256.Sum256(encoded)
	return dashboardProjectListCachePrefix + ":" + hex.EncodeToString(sum[:])
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
