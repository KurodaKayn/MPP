package services

import (
	"encoding/json"
	"math"

	"github.com/google/uuid"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/dto"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"gorm.io/gorm"
)

type DashboardService struct {
	db *gorm.DB
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return &DashboardService{db: db}
}

func (s *DashboardService) GetStats() (*dto.DashboardStatsResponse, error) {
	var stats dto.DashboardStatsResponse

	if err := s.db.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&models.Project{}).Count(&stats.TotalProjects).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&models.ProjectPlatformPublication{}).
		Where("status = ?", models.PublicationStatusPublished).
		Count(&stats.TotalPublishedPublications).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&models.ProjectPlatformPublication{}).
		Where("status = ?", models.PublicationStatusFailed).
		Count(&stats.TotalFailedPublications).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *DashboardService) ListProjects(page, limit int, status, userID, platform string) (*dto.PaginationResponse, error) {
	var projects []models.Project
	var total int64

	query := s.db.Model(&models.Project{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if userID != "" {
		if uid, err := uuid.Parse(userID); err == nil {
			query = query.Where("user_id = ?", uid)
		}
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
		Order("created_at desc").
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

func (s *DashboardService) GetProjectPublications(projectID uuid.UUID) (*dto.ProjectPublicationsResponse, error) {
	// Verify project exists
	var count int64
	if err := s.db.Model(&models.Project{}).Where("id = ?", projectID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, gorm.ErrRecordNotFound
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
