package stats

import (
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
	if cached, err := s.cache.Get(ctx, dashboardStatsCacheKey).Result(); err == nil {
		var stats dto.DashboardStatsResponse
		if err := json.Unmarshal([]byte(cached), &stats); err == nil {
			return &stats, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return s.computeStats(nil)
	}

	stats, err := s.computeStats(nil)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return stats, nil
	}
	if err := s.cache.Set(ctx, dashboardStatsCacheKey, encoded, s.cacheTTL).Err(); err != nil {
		return stats, nil
	}
	return stats, nil
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
