package platformaccount

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
)

const dashboardAccountCachePrefix = "mpp:dashboard:accounts:v1"
const dashboardAccountCacheVersion = 1
const dashboardAccountRefreshTimeout = 15 * time.Second

type dashboardAccountCachePayload[T any] struct {
	Version     int       `json:"version"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Platform    string    `json:"platform"`
	Response    T         `json:"response"`
}

func getCachedDashboardAccount[T any](s *Service, userID uuid.UUID, workspaceID uuid.UUID, platform string, compute func(*Service, uuid.UUID) (*T, error), valid func(T) bool) (*T, error) {
	workspaceID = s.WorkspaceIDForUser(userID, workspaceID)
	if !s.canUseDashboardAccountCache() {
		return compute(s, workspaceID)
	}

	ctx := s.requestContext()
	cacheKey := dashboardAccountCacheKey(workspaceID, platform)
	if resp, hit, err := cachedDashboardAccount(ctx, s, cacheKey, workspaceID, platform, valid); hit {
		return resp, nil
	} else if err != nil {
		return compute(s, workspaceID)
	}

	if s.cacheGroup == nil {
		return refreshDashboardAccountCache(ctx, s, cacheKey, workspaceID, platform, compute)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardAccountRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if resp, hit, err := cachedDashboardAccount(refreshCtx, refreshSvc, cacheKey, workspaceID, platform, valid); hit {
			return resp, nil
		} else if err != nil {
			return compute(refreshSvc, workspaceID)
		}
		return refreshDashboardAccountCache(refreshCtx, refreshSvc, cacheKey, workspaceID, platform, compute)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*T); ok {
			return resp, nil
		}
		return compute(s, workspaceID)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func cachedDashboardAccount[T any](ctx context.Context, s *Service, cacheKey string, workspaceID uuid.UUID, platform string, valid func(T) bool) (*T, bool, error) {
	cached, err := s.cache.Get(ctx, cacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if resp, ok := decodeDashboardAccountPayload(cached, workspaceID, platform, valid); ok {
		return resp, true, nil
	}
	return nil, false, nil
}

func decodeDashboardAccountPayload[T any](cached []byte, workspaceID uuid.UUID, platform string, valid func(T) bool) (*T, bool) {
	var payload dashboardAccountCachePayload[T]
	if err := json.Unmarshal(cached, &payload); err != nil {
		return nil, false
	}
	if !dashboardAccountPayloadValid(payload, workspaceID, platform, valid) {
		return nil, false
	}
	return &payload.Response, true
}

func refreshDashboardAccountCache[T any](ctx context.Context, s *Service, cacheKey string, workspaceID uuid.UUID, platform string, compute func(*Service, uuid.UUID) (*T, error)) (*T, error) {
	resp, err := compute(s, workspaceID)
	if err != nil {
		return nil, err
	}
	payload := dashboardAccountCachePayload[T]{
		Version:     dashboardAccountCacheVersion,
		WorkspaceID: workspaceID,
		Platform:    platform,
		Response:    *resp,
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = s.cache.Set(ctx, cacheKey, encoded, s.cacheTTL).Err()
	}
	return resp, nil
}

func (s *Service) invalidateDashboardAccountCache(workspaceID uuid.UUID, platform string) {
	if s.cache == nil {
		return
	}
	_ = s.cache.Del(s.requestContext(), dashboardAccountCacheKey(workspaceID, platform)).Err()
}

func (s *Service) canUseDashboardAccountCache() bool {
	if s.cache == nil || s.cacheTTL <= 0 {
		return false
	}
	stickyUntil, sticky := dbrouter.StickyWriterUntil(s.requestContext())
	return !sticky || !stickyUntil.After(time.Now())
}

func dashboardAccountCacheKey(workspaceID uuid.UUID, platform string) string {
	return dashboardAccountCachePrefix + ":" + workspaceID.String() + ":" + platform
}

func dashboardAccountRefreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardAccountRefreshTimeout)
}

func dashboardAccountPayloadValid[T any](payload dashboardAccountCachePayload[T], workspaceID uuid.UUID, platform string, valid func(T) bool) bool {
	if payload.Version != dashboardAccountCacheVersion {
		return false
	}
	if payload.WorkspaceID != workspaceID || payload.Platform != platform {
		return false
	}
	return valid(payload.Response)
}
