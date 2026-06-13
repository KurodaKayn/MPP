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

const dashboardStatsCachePrefix = "mpp:dashboard:stats:global:v1"
const dashboardStatsCacheGenerationKey = "mpp:dashboard:stats-generation:v1"
const dashboardStatsCacheVersion = 1
const dashboardStatsRefreshTimeout = 15 * time.Second
const dashboardStatsInvalidateTimeout = 2 * time.Second

type dashboardStatsCachePayload struct {
	Version    int                        `json:"version"`
	Generation string                     `json:"generation"`
	Stats      dto.DashboardStatsResponse `json:"stats"`
}

func (s *Service) GetStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	if scopeUserID == nil && s.canUseDashboardStatsCache() {
		return s.getCachedStats()
	}
	return s.computeStats(scopeUserID)
}

func (s *Service) computeStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	if scopeUserID == nil && s.canUseReadModels() {
		if stats, ok, err := s.globalStatsFromReadModel(); err != nil {
			return nil, err
		} else if ok {
			return stats, nil
		}
	}

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

func (s *Service) globalStatsFromReadModel() (*dto.DashboardStatsResponse, bool, error) {
	readDB := s.eventualReadDB()
	var workspaceCount int64
	if err := readDB.Model(&models.Workspace{}).Count(&workspaceCount).Error; err != nil {
		return nil, false, err
	}
	if workspaceCount == 0 {
		return nil, false, nil
	}
	var statsRows int64
	if err := readDB.Model(&models.WorkspaceDashboardStats{}).Count(&statsRows).Error; err != nil {
		return nil, false, err
	}
	if statsRows != workspaceCount {
		return nil, false, nil
	}

	var totals struct {
		TotalProjects              int64
		TotalPublishedPublications int64
		TotalFailedPublications    int64
	}
	if err := readDB.Model(&models.WorkspaceDashboardStats{}).
		Select(`
			COALESCE(SUM(total_projects), 0) AS total_projects,
			COALESCE(SUM(total_published_publications), 0) AS total_published_publications,
			COALESCE(SUM(total_failed_publications), 0) AS total_failed_publications
		`).
		Scan(&totals).Error; err != nil {
		return nil, false, err
	}

	stats := dto.DashboardStatsResponse{
		TotalProjects:              totals.TotalProjects,
		TotalPublishedPublications: totals.TotalPublishedPublications,
		TotalFailedPublications:    totals.TotalFailedPublications,
	}
	if err := readDB.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, false, err
	}
	return &stats, true, nil
}

func (s *Service) getCachedStats() (*dto.DashboardStatsResponse, error) {
	ctx := s.requestContext()
	generation, err := s.dashboardStatsCacheGeneration(ctx)
	cacheUnavailable := err != nil
	if cacheUnavailable {
		generation = "0"
	}
	cacheKey := dashboardStatsCacheKey(generation)
	if !cacheUnavailable {
		if stats, hit, _ := s.cachedDashboardStats(ctx, cacheKey, generation); hit {
			return stats, nil
		}
	}

	if s.cacheGroup == nil {
		if cacheUnavailable {
			return s.computeStats(nil)
		}
		return s.refreshDashboardStatsCache(ctx, cacheKey, generation)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardStatsRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if cacheUnavailable {
			return refreshSvc.computeStats(nil)
		}
		if stats, hit, err := refreshSvc.cachedDashboardStats(refreshCtx, cacheKey, generation); hit {
			return stats, nil
		} else if err != nil {
			return refreshSvc.computeStats(nil)
		}
		return refreshSvc.refreshDashboardStatsCache(refreshCtx, cacheKey, generation)
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

func (s *Service) cachedDashboardStats(ctx context.Context, cacheKey string, generation string) (*dto.DashboardStatsResponse, bool, error) {
	cached, err := s.cache.Get(ctx, cacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if stats, ok := decodeDashboardStatsCachePayload(cached, generation); ok {
		return stats, true, nil
	}
	return nil, false, nil
}

func decodeDashboardStatsCachePayload(cached []byte, generation string) (*dto.DashboardStatsResponse, bool) {
	var payload dashboardStatsCachePayload
	if err := json.Unmarshal(cached, &payload); err != nil {
		return nil, false
	}
	if !dashboardStatsPayloadValid(payload, generation) {
		return nil, false
	}
	return &payload.Stats, true
}

func (s *Service) refreshDashboardStatsCache(ctx context.Context, cacheKey string, generation string) (*dto.DashboardStatsResponse, error) {
	stats, err := s.computeStats(nil)
	if err != nil {
		return nil, err
	}
	payload := dashboardStatsCachePayload{
		Version:    dashboardStatsCacheVersion,
		Generation: generation,
		Stats:      *stats,
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = s.cache.Set(ctx, cacheKey, encoded, s.cacheTTL).Err()
	}
	return stats, nil
}

func dashboardStatsRefreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardStatsRefreshTimeout)
}

func dashboardStatsPayloadValid(payload dashboardStatsCachePayload, generation string) bool {
	if payload.Version != dashboardStatsCacheVersion {
		return false
	}
	if payload.Generation != generation {
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
	if s.canUseReadModels() {
		var readModel models.WorkspaceDashboardStats
		if err := s.eventualReadDB().First(&readModel, "workspace_id = ?", workspaceID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		} else {
			return &dto.DashboardStatsResponse{
				TotalUsers:                 1,
				TotalProjects:              readModel.TotalProjects,
				TotalPublishedPublications: readModel.TotalPublishedPublications,
				TotalFailedPublications:    readModel.TotalFailedPublications,
			}, nil
		}
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
	return s.canUseReadModels()
}

func (s *Service) canUseReadModels() bool {
	stickyUntil, sticky := dbrouter.StickyWriterUntil(s.requestContext())
	return !sticky || !stickyUntil.After(time.Now())
}

func (s *Service) InvalidateDashboardStatsCache(ctx context.Context) {
	if s == nil || s.cache == nil {
		return
	}
	if ctx != nil {
		s = s.WithContext(ctx)
	}
	invalidateCtx, cancel := dashboardStatsInvalidationContext(s.requestContext())
	defer cancel()
	_ = s.cache.Incr(invalidateCtx, dashboardStatsCacheGenerationKey).Err()
	deleteDashboardStatsCacheKeys(invalidateCtx, s.cache)
}

func dashboardStatsInvalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardStatsInvalidateTimeout)
}

func (s *Service) dashboardStatsCacheGeneration(ctx context.Context) (string, error) {
	generation, err := s.cache.Get(ctx, dashboardStatsCacheGenerationKey).Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return generation, err
}

func dashboardStatsCacheKey(generation string) string {
	return dashboardStatsCachePrefix + ":" + generation
}

func deleteDashboardStatsCacheKeys(ctx context.Context, client *redis.Client) {
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, dashboardStatsCachePrefix+":*", 100).Result()
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
