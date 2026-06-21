package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	projectlisting "github.com/kurodakayn/mpp-backend/internal/services/project/listing"
)

func (s *Service) projectListCache() *projectlisting.Cache {
	if s == nil {
		return nil
	}
	return projectlisting.New(projectlisting.Config{
		Client: s.cache,
		TTL:    s.cacheTTL,
		Group:  s.cacheGroup,
		Guard:  s.projectListGuard,
	})
}

func (s *Service) getCachedDashboardProjectList(cursor string, page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	params := projectlisting.Params{
		Cursor:       cursor,
		Page:         page,
		Limit:        limit,
		Status:       status,
		FilterUserID: filterUserID,
		Platform:     platform,
		ScopeUserID:  projectlisting.UUIDStringValue(scopeUserID),
	}
	return s.projectListCache().Get(s.requestContext(), params, func(ctx context.Context) (*dto.PaginationResponse, error) {
		return s.WithContext(ctx).computeProjectList(cursor, page, limit, status, filterUserID, platform, scopeUserID)
	})
}

func (s *Service) getCachedWorkspaceProjectList(workspaceID uuid.UUID, actorUserID uuid.UUID, cursor string, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	params := projectlisting.Params{
		Cursor:      cursor,
		Page:        page,
		Limit:       limit,
		Status:      status,
		Platform:    platform,
		WorkspaceID: workspaceID.String(),
		ActorUserID: actorUserID.String(),
	}
	return s.projectListCache().Get(s.requestContext(), params, func(ctx context.Context) (*dto.PaginationResponse, error) {
		return s.WithContext(ctx).computeWorkspaceProjectList(workspaceID, actorUserID, cursor, page, limit, status, platform)
	})
}

func (s *Service) canUseDashboardProjectListCache() bool {
	if s == nil {
		return false
	}
	return s.projectListCache().CanUse(s.requestContext())
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
	if s == nil {
		return
	}
	s.projectListCache().Invalidate(s.requestContext())
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
