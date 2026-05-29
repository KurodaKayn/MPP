package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/dto"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/publisher"
	"gorm.io/gorm"
)

// ...

func (s *DashboardService) PublishProject(projectID uuid.UUID, platform string, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	// 1. Check ownership
	var proj models.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, *scopeUserID).First(&proj).Error; err != nil {
		return nil, ErrForbidden
	}

	// 2. Get publication record
	var pub models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platform).First(&pub).Error; err != nil {
		return nil, fmt.Errorf("publication record not found for platform: %s", platform)
	}

	if err := s.applySavedWechatCredentialsToPublication(proj.UserID, &pub); err != nil {
		return nil, err
	}

	// 3. Get Publisher from factory
	p, err := publisher.Factory.GetPublisher(platform)
	if err != nil {
		return nil, err
	}

	// 4. Execute Publish
	// Note: We use pub.AdaptedContent. If empty, you might want to sync from proj.SourceContent first.
	remoteID, publishURL, err := p.Publish(context.Background(), &pub)

	// 5. Update DB
	status := models.PublicationStatusPublished
	errMsg := ""
	if err != nil {
		status = models.PublicationStatusFailed
		errMsg = err.Error()
	}

	updates := map[string]interface{}{
		"status":        status,
		"remote_id":     remoteID,
		"publish_url":   publishURL,
		"error_message": errMsg,
	}
	s.db.Model(&pub).Updates(updates)

	return updates, nil
}

var ErrForbidden = errors.New("forbidden: you do not have permission to access this resource")

type DashboardService struct {
	db           *gorm.DB
	wechatTester WechatConnectionTester
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return NewDashboardServiceWithWechatTester(db, WechatAPITester{})
}

func NewDashboardServiceWithWechatTester(db *gorm.DB, tester WechatConnectionTester) *DashboardService {
	if tester == nil {
		tester = WechatAPITester{}
	}
	return &DashboardService{db: db, wechatTester: tester}
}

func (s *DashboardService) GetStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	var stats dto.DashboardStatsResponse

	// Users count (Only admin should see total users)
	if scopeUserID == nil {
		if err := s.db.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
			return nil, err
		}
	} else {
		stats.TotalUsers = 1 // Scoped to self
	}

	// Projects count
	projQuery := s.db.Model(&models.Project{})
	if scopeUserID != nil {
		projQuery = projQuery.Where("user_id = ?", *scopeUserID)
	}
	if err := projQuery.Count(&stats.TotalProjects).Error; err != nil {
		return nil, err
	}

	// Published publications count
	pubPubQuery := s.db.Model(&models.ProjectPlatformPublication{}).Where("project_platform_publications.status = ?", models.PublicationStatusPublished)
	if scopeUserID != nil {
		pubPubQuery = pubPubQuery.Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
			Where("projects.user_id = ?", *scopeUserID)
	}
	if err := pubPubQuery.Count(&stats.TotalPublishedPublications).Error; err != nil {
		return nil, err
	}

	// Failed publications count
	failPubQuery := s.db.Model(&models.ProjectPlatformPublication{}).Where("project_platform_publications.status = ?", models.PublicationStatusFailed)
	if scopeUserID != nil {
		failPubQuery = failPubQuery.Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
			Where("projects.user_id = ?", *scopeUserID)
	}
	if err := failPubQuery.Count(&stats.TotalFailedPublications).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *DashboardService) ListProjects(page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	var projects []models.Project
	var total int64

	query := s.db.Model(&models.Project{})

	// Apply scope (User dashboard enforces scopeUserID, overriding any filterUserID)
	if scopeUserID != nil {
		query = query.Where("user_id = ?", *scopeUserID)
	} else if filterUserID != "" {
		// Admin dashboard can filter by specific user
		if uid, err := uuid.Parse(filterUserID); err == nil {
			query = query.Where("user_id = ?", uid)
		}
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if platform != "" {
		query = query.Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id").
			Where("ppp.platform = ?", platform).
			Group("projects.id")
	}

	// Count total before pagination
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// Calculate pagination
	offset := (page - 1) * limit
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// Fetch data with specific fields and preload summary publications
	if err := query.Omit("source_content").
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, publish_url")
		}).
		Order("projects.created_at desc").
		Limit(limit).Offset(offset).
		Find(&projects).Error; err != nil {
		return nil, err
	}

	// Map to DTO
	var items []dto.ProjectListItem
	for _, p := range projects {
		var pubSummaries []dto.PublicationSummary
		for _, pub := range p.Publications {
			pubSummaries = append(pubSummaries, dto.PublicationSummary{
				ID:         pub.ID,
				Platform:   pub.Platform,
				Enabled:    pub.Enabled,
				Status:     pub.Status,
				PublishURL: pub.PublishURL,
			})
		}

		items = append(items, dto.ProjectListItem{
			ID:           p.ID,
			UserID:       p.UserID,
			Title:        p.Title,
			Status:       p.Status,
			CreatedAt:    p.CreatedAt,
			UpdatedAt:    p.UpdatedAt,
			Publications: pubSummaries,
		})
	}

	if items == nil {
		items = []dto.ProjectListItem{} // ensure empty array instead of null
	}

	return &dto.PaginationResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *DashboardService) GetProjectPublications(projectID uuid.UUID, scopeUserID *uuid.UUID) (*dto.ProjectPublicationsResponse, error) {
	// Verify project exists and ownership
	var proj models.Project
	if err := s.db.Select("id, user_id").Where("id = ?", projectID).First(&proj).Error; err != nil {
		return nil, err
	}

	// Enforce ownership if scoped
	if scopeUserID != nil && proj.UserID != *scopeUserID {
		return nil, ErrForbidden
	}

	var publications []models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ?", projectID).Find(&publications).Error; err != nil {
		return nil, err
	}

	var items []dto.PublicationDetail
	for _, pub := range publications {
		// Safe parse config
		var rawConfig map[string]interface{}
		_ = json.Unmarshal(pub.Config, &rawConfig)
		safeConfig := filterConfig(rawConfig)

		// Safe parse adapted content (summary)
		var rawContent map[string]interface{}
		_ = json.Unmarshal(pub.AdaptedContent, &rawContent)
		safeContent := summarizeAdaptedContent(rawContent)

		items = append(items, dto.PublicationDetail{
			ID:             pub.ID,
			Platform:       pub.Platform,
			Enabled:        pub.Enabled,
			Status:         pub.Status,
			ErrorMessage:   pub.ErrorMessage,
			Config:         safeConfig,
			AdaptedContent: safeContent,
			PublishURL:     pub.PublishURL,
			RemoteID:       pub.RemoteID,
			RetryCount:     pub.RetryCount,
			LastAttemptAt:  pub.LastAttemptAt,
			PublishedAt:    pub.PublishedAt,
			CreatedAt:      pub.CreatedAt,
			UpdatedAt:      pub.UpdatedAt,
		})
	}

	if items == nil {
		items = []dto.PublicationDetail{}
	}

	return &dto.ProjectPublicationsResponse{
		ProjectID: projectID,
		Items:     items,
	}, nil
}

// Helper functions to filter sensitive data from JSONB fields

func filterConfig(raw map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{})
	allowedKeys := []string{"title", "tags", "cover_image", "topics", "category", "original_declaration"}
	for _, key := range allowedKeys {
		if val, ok := raw[key]; ok {
			safe[key] = val
		}
	}
	return safe
}

func summarizeAdaptedContent(raw map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{})
	if summary, ok := raw["summary"]; ok {
		safe["summary"] = summary
	} else {
		safe["summary"] = "Content adapted (no summary available)"
	}
	if format, ok := raw["format"]; ok {
		safe["format"] = format
	}
	return safe
}
