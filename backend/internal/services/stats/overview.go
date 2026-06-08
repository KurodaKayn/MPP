package stats

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const dashboardStatsCacheKey = "mpp:dashboard:stats:global:v1"
const dashboardStatsCacheVersion = 1
const dashboardStatsRefreshTimeout = 15 * time.Second

type dashboardStatsCachePayload struct {
	Version int                        `json:"version"`
	Stats   dto.DashboardStatsResponse `json:"stats"`
}

func (s *Service) GetStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	if scopeUserID == nil && s.canUseDashboardStatsCache() {
		return s.getCachedStats()
	}
	return s.computeStats(scopeUserID)
}

func (s *Service) computeStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	var stats dto.DashboardStatsResponse
	readDB := s.statsReadDB(scopeUserID)

	// Users count (Only admin should see total users)
	if scopeUserID == nil {
		if err := readDB.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
			return nil, err
		}
	} else {
		stats.TotalUsers = 1 // Scoped to self
	}

	// Projects count
	projQuery := readDB.Model(&models.Project{})
	if scopeUserID != nil {
		projQuery = s.projects.ScopeAccessibleProjects(projQuery, *scopeUserID)
	}
	if err := projQuery.Count(&stats.TotalProjects).Error; err != nil {
		return nil, err
	}

	// Published publications count
	pubPubQuery := readDB.Model(&models.ProjectPlatformPublication{}).Where("project_platform_publications.status = ?", models.PublicationStatusPublished)
	if scopeUserID != nil {
		pubPubQuery = pubPubQuery.Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
			Scopes(func(db *gorm.DB) *gorm.DB {
				return s.projects.ScopeAccessibleProjects(db, *scopeUserID)
			})
	}
	if err := pubPubQuery.Count(&stats.TotalPublishedPublications).Error; err != nil {
		return nil, err
	}

	// Failed publications count
	failPubQuery := readDB.Model(&models.ProjectPlatformPublication{}).Where("project_platform_publications.status = ?", models.PublicationStatusFailed)
	if scopeUserID != nil {
		failPubQuery = failPubQuery.Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
			Scopes(func(db *gorm.DB) *gorm.DB {
				return s.projects.ScopeAccessibleProjects(db, *scopeUserID)
			})
	}
	if err := failPubQuery.Count(&stats.TotalFailedPublications).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *Service) getCachedStats() (*dto.DashboardStatsResponse, error) {
	ctx := s.requestContext()
	if stats, hit, _ := s.cachedDashboardStats(ctx); hit {
		return stats, nil
	}

	if s.cacheGroup == nil {
		return s.refreshDashboardStatsCache(ctx)
	}

	resultCh := s.cacheGroup.DoChan(dashboardStatsCacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardStatsRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if stats, hit, err := refreshSvc.cachedDashboardStats(refreshCtx); hit {
			return stats, nil
		} else if err != nil {
			return refreshSvc.computeStats(nil)
		}
		return refreshSvc.refreshDashboardStatsCache(refreshCtx)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if stats, ok := result.Val.(*dto.DashboardStatsResponse); ok {
			return stats, nil
		}
		return s.computeStats(nil)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) cachedDashboardStats(ctx context.Context) (*dto.DashboardStatsResponse, bool, error) {
	cached, err := s.cache.Get(ctx, dashboardStatsCacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if stats, ok := decodeDashboardStatsCachePayload(cached); ok {
		return stats, true, nil
	}
	return nil, false, nil
}

func decodeDashboardStatsCachePayload(cached []byte) (*dto.DashboardStatsResponse, bool) {
	var payload dashboardStatsCachePayload
	if err := json.Unmarshal(cached, &payload); err != nil {
		return nil, false
	}
	if !dashboardStatsPayloadValid(payload) {
		return nil, false
	}
	return &payload.Stats, true
}

func (s *Service) refreshDashboardStatsCache(ctx context.Context) (*dto.DashboardStatsResponse, error) {
	stats, err := s.computeStats(nil)
	if err != nil {
		return nil, err
	}
	payload := dashboardStatsCachePayload{
		Version: dashboardStatsCacheVersion,
		Stats:   *stats,
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = s.cache.Set(ctx, dashboardStatsCacheKey, encoded, s.cacheTTL).Err()
	}
	return stats, nil
}

func dashboardStatsRefreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardStatsRefreshTimeout)
}

func dashboardStatsPayloadValid(payload dashboardStatsCachePayload) bool {
	if payload.Version != dashboardStatsCacheVersion {
		return false
	}
	return payload.Stats.TotalUsers >= 0 &&
		payload.Stats.TotalProjects >= 0 &&
		payload.Stats.TotalPublishedPublications >= 0 &&
		payload.Stats.TotalFailedPublications >= 0
}

func (s *Service) GetWorkspaceStats(workspaceID uuid.UUID, scopeUserID uuid.UUID) (*dto.DashboardStatsResponse, error) {
	if _, err := s.projects.WorkspaceProjectRole(workspaceID, scopeUserID); err != nil {
		return nil, err
	}

	var stats dto.DashboardStatsResponse
	stats.TotalUsers = 1
	readDB := s.strongReadDB()

	projQuery := readDB.Model(&models.Project{}).Where("workspace_id = ?", workspaceID)
	if err := projQuery.Count(&stats.TotalProjects).Error; err != nil {
		return nil, err
	}

	pubQuery := readDB.Model(&models.ProjectPlatformPublication{}).
		Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
		Where("projects.workspace_id = ?", workspaceID)
	if err := pubQuery.Where("project_platform_publications.status = ?", models.PublicationStatusPublished).
		Count(&stats.TotalPublishedPublications).Error; err != nil {
		return nil, err
	}

	pubQuery = readDB.Model(&models.ProjectPlatformPublication{}).
		Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
		Where("projects.workspace_id = ?", workspaceID)
	if err := pubQuery.Where("project_platform_publications.status = ?", models.PublicationStatusFailed).
		Count(&stats.TotalFailedPublications).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *Service) statsReadDB(scopeUserID *uuid.UUID) *gorm.DB {
	if scopeUserID != nil {
		return s.strongReadDB()
	}
	return s.eventualReadDB()
}

func (s *Service) canUseDashboardStatsCache() bool {
	if s.cache == nil || s.cacheTTL <= 0 {
		return false
	}
	stickyUntil, sticky := dbrouter.StickyWriterUntil(s.requestContext())
	return !sticky || !stickyUntil.After(time.Now())
}
