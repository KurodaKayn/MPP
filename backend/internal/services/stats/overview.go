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
	"github.com/kurodakayn/mpp-backend/internal/pkg/cachettl"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
)

const dashboardStatsCachePrefix = "mpp:dashboard:stats:global:v1"
const dashboardUserStatsCachePrefix = "mpp:dashboard:stats:user:v1"
const dashboardWorkspaceStatsCachePrefix = "mpp:dashboard:stats:workspace:v1"
const dashboardStatsCacheGenerationKey = "mpp:dashboard:stats-generation:v1"
const dashboardScopedStatsCacheGenerationKey = "mpp:dashboard:stats-generation:scoped:v1"
const dashboardStatsCacheVersion = 1
const dashboardStatsRefreshTimeout = 15 * time.Second
const dashboardStatsInvalidateTimeout = 2 * time.Second

type dashboardStatsCachePayload struct {
	Version    int                        `json:"version"`
	Generation string                     `json:"generation"`
	Stats      dto.DashboardStatsResponse `json:"stats"`
}

type dashboardStatsCacheCompute func(*Service) (*dto.DashboardStatsResponse, error)

func (s *Service) GetStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	if scopeUserID == nil && s.canUseDashboardStatsCache() {
		return s.getCachedStats()
	}
	if scopeUserID != nil && s.canUseDashboardStatsCache() {
		return s.getCachedUserStats(*scopeUserID)
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

	var factProjectsCount int64
	if err := readDB.Model(&models.Project{}).Count(&factProjectsCount).Error; err != nil {
		return nil, false, err
	}
	if totals.TotalProjects != factProjectsCount {
		return nil, false, nil
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
	return s.getCachedDashboardStats(dashboardStatsCacheGenerationKey, dashboardStatsCacheKey, func(svc *Service) (*dto.DashboardStatsResponse, error) {
		return svc.computeStats(nil)
	})
}

func (s *Service) getCachedUserStats(userID uuid.UUID) (*dto.DashboardStatsResponse, error) {
	return s.getCachedDashboardStats(dashboardScopedStatsCacheGenerationKey, func(generation string) string {
		return dashboardUserStatsCacheKey(userID, generation)
	}, func(svc *Service) (*dto.DashboardStatsResponse, error) {
		return svc.computeStats(&userID)
	})
}

func (s *Service) getCachedWorkspaceStats(workspaceID uuid.UUID) (*dto.DashboardStatsResponse, error) {
	return s.getCachedDashboardStats(dashboardScopedStatsCacheGenerationKey, func(generation string) string {
		return dashboardWorkspaceStatsCacheKey(workspaceID, generation)
	}, func(svc *Service) (*dto.DashboardStatsResponse, error) {
		return svc.computeWorkspaceStats(workspaceID)
	})
}

func (s *Service) getCachedDashboardStats(generationKey string, cacheKeyForGeneration func(string) string, compute dashboardStatsCacheCompute) (*dto.DashboardStatsResponse, error) {
	ctx := s.requestContext()
	generation, err := s.dashboardStatsCacheGeneration(ctx, generationKey)
	cacheUnavailable := err != nil
	if cacheUnavailable {
		generation = "0"
	}
	cacheKey := cacheKeyForGeneration(generation)
	if !cacheUnavailable {
		if stats, hit, _ := s.cachedDashboardStats(ctx, cacheKey, generation); hit {
			return stats, nil
		}
	}

	if s.cacheGroup == nil {
		if cacheUnavailable {
			return compute(s)
		}
		return s.refreshDashboardStatsCache(ctx, cacheKey, generation, compute)
	}

	resultCh := s.cacheGroup.DoChan(cacheKey, func() (any, error) {
		refreshCtx, cancel := dashboardStatsRefreshContext(ctx)
		defer cancel()
		refreshSvc := s.WithContext(refreshCtx)
		if cacheUnavailable {
			return compute(refreshSvc)
		}
		if stats, hit, err := refreshSvc.cachedDashboardStats(refreshCtx, cacheKey, generation); hit {
			return stats, nil
		} else if err != nil {
			return compute(refreshSvc)
		}
		return refreshSvc.refreshDashboardStatsCache(refreshCtx, cacheKey, generation, compute)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if stats, ok := result.Val.(*dto.DashboardStatsResponse); ok {
			return stats, nil
		}
		return compute(s)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) cachedDashboardStats(ctx context.Context, cacheKey string, generation string) (*dto.DashboardStatsResponse, bool, error) {
	cached, err := redisdegrade.Call(s.cacheGuard, func() ([]byte, error) {
		return s.cache.Get(ctx, cacheKey).Bytes()
	})
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

func (s *Service) refreshDashboardStatsCache(ctx context.Context, cacheKey string, generation string, compute dashboardStatsCacheCompute) (*dto.DashboardStatsResponse, error) {
	stats, err := compute(s)
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
		_ = redisdegrade.Do(s.cacheGuard, func() error {
			return s.cache.Set(ctx, cacheKey, encoded, cachettl.Jitter(s.cacheTTL, cacheKey)).Err()
		})
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
	if s.canUseDashboardStatsCache() {
		return s.getCachedWorkspaceStats(workspaceID)
	}
	return s.computeWorkspaceStats(workspaceID)
}

func (s *Service) computeWorkspaceStats(workspaceID uuid.UUID) (*dto.DashboardStatsResponse, error) {
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
	where, args, err := s.workspaceProjectWhere(readDB, workspaceID)
	if err != nil {
		return nil, err
	}

	projQuery := readDB.Model(&models.Project{}).Where(where, args...)
	if err := projQuery.Count(&stats.TotalProjects).Error; err != nil {
		return nil, err
	}

	pubQuery := readDB.Model(&models.ProjectPlatformPublication{}).
		Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
		Where(where, args...)
	if err := pubQuery.Where("project_platform_publications.status = ?", models.PublicationStatusPublished).
		Count(&stats.TotalPublishedPublications).Error; err != nil {
		return nil, err
	}

	pubQuery = readDB.Model(&models.ProjectPlatformPublication{}).
		Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
		Where(where, args...)
	if err := pubQuery.Where("project_platform_publications.status = ?", models.PublicationStatusFailed).
		Count(&stats.TotalFailedPublications).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *Service) workspaceProjectWhere(readDB *gorm.DB, workspaceID uuid.UUID) (string, []any, error) {
	var workspace models.Workspace
	err := readDB.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "projects.workspace_id = ?", []any{workspaceID}, nil
	}
	if err != nil {
		return "", nil, err
	}
	if workspace.OwnerUserID != uuid.Nil && workspaceID == models.PersonalWorkspaceID(workspace.OwnerUserID) {
		return "(projects.workspace_id = ? OR (projects.workspace_id IS NULL AND projects.user_id = ?))", []any{workspaceID, workspace.OwnerUserID}, nil
	}
	return "projects.workspace_id = ?", []any{workspaceID}, nil
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
	s.incrementDashboardStatsGeneration(invalidateCtx, dashboardStatsCacheGenerationKey)
	s.incrementDashboardStatsGeneration(invalidateCtx, dashboardScopedStatsCacheGenerationKey)
}

func (s *Service) InvalidateDashboardScopedStatsCache(ctx context.Context) {
	if s == nil || s.cache == nil {
		return
	}
	if ctx != nil {
		s = s.WithContext(ctx)
	}
	invalidateCtx, cancel := dashboardStatsInvalidationContext(s.requestContext())
	defer cancel()
	s.incrementDashboardStatsGeneration(invalidateCtx, dashboardScopedStatsCacheGenerationKey)
}

func (s *Service) incrementDashboardStatsGeneration(ctx context.Context, generationKey string) {
	_ = redisdegrade.Do(s.cacheGuard, func() error {
		return s.cache.Incr(ctx, generationKey).Err()
	})
}

func dashboardStatsInvalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), dashboardStatsInvalidateTimeout)
}

func (s *Service) dashboardStatsCacheGeneration(ctx context.Context, generationKey string) (string, error) {
	generation, err := redisdegrade.Call(s.cacheGuard, func() (string, error) {
		return s.cache.Get(ctx, generationKey).Result()
	})
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return generation, err
}

func dashboardStatsCacheKey(generation string) string {
	return dashboardStatsCachePrefix + ":" + generation
}

func dashboardUserStatsCacheKey(userID uuid.UUID, generation string) string {
	return dashboardUserStatsCachePrefix + ":" + userID.String() + ":" + generation
}

func dashboardWorkspaceStatsCacheKey(workspaceID uuid.UUID, generation string) string {
	return dashboardWorkspaceStatsCachePrefix + ":" + workspaceID.String() + ":" + generation
}
