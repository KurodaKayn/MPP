package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"gorm.io/datatypes"
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
	if !pub.Enabled || pub.Status == models.PublicationStatusDisabled {
		return nil, ErrPublicationDisabled
	}

	if err := s.applySavedWechatCredentialsToPublication(proj.UserID, &pub); err != nil {
		return nil, err
	}
	if err := s.applySavedXCredentialsToPublication(proj.UserID, &pub); err != nil {
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
		errMsg = sanitizeUserFacingErrorMessage(err.Error())
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
var ErrInvalidProject = errors.New("invalid project")
var ErrPublicationDisabled = errors.New("publication is disabled")

var sensitiveErrorQueryParamPattern = regexp.MustCompile(`(?i)(secret|access_token)=([^&"\s]+)`)
var allowedProjectPlatforms = map[string]struct{}{
	"bilibili":    {},
	"wechat":      {},
	"x":           {},
	"xiaohongshu": {},
	"zhihu":       {},
}

func sanitizeUserFacingErrorMessage(message string) string {
	return sensitiveErrorQueryParamPattern.ReplaceAllString(message, "$1=<redacted>")
}

type DashboardService struct {
	db           *gorm.DB
	wechatTester WechatConnectionTester
	xTester      XConnectionTester
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, WechatAPITester{}, XAPITester{})
}

func NewDashboardServiceWithWechatTester(db *gorm.DB, tester WechatConnectionTester) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, tester, XAPITester{})
}

func NewDashboardServiceWithPlatformTesters(db *gorm.DB, tester WechatConnectionTester, xTester XConnectionTester) *DashboardService {
	if tester == nil {
		tester = WechatAPITester{}
	}
	if xTester == nil {
		xTester = XAPITester{}
	}
	return &DashboardService{db: db, wechatTester: tester, xTester: xTester}
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

func (s *DashboardService) CreateProject(userID uuid.UUID, req dto.CreateProjectRequest) (*dto.ProjectListItem, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := strings.TrimSpace(req.SourceContent)
	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if title == "" || sourceContent == "" || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	project := models.Project{
		UserID:        userID,
		Title:         title,
		SourceContent: sourceContent,
		Status:        models.ProjectStatusReady,
	}
	publications := make([]dto.PublicationSummary, 0, len(platforms))

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		for _, platform := range platforms {
			config, adaptedContent, status, err := buildPublicationPayload(&project, platform, title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}

			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}

			publications = append(publications, dto.PublicationSummary{
				ID:       publication.ID,
				Platform: platform,
				Enabled:  publication.Enabled,
				Status:   publication.Status,
			})
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &dto.ProjectListItem{
		ID:           project.ID,
		UserID:       project.UserID,
		Title:        project.Title,
		Status:       project.Status,
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    project.UpdatedAt,
		Publications: publications,
	}, nil
}

func (s *DashboardService) GetProject(projectID uuid.UUID, scopeUserID *uuid.UUID) (*dto.ProjectDetail, error) {
	var project models.Project
	if err := s.db.
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, publish_url").Order("platform asc")
		}).
		First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}

	if scopeUserID != nil && project.UserID != *scopeUserID {
		return nil, ErrForbidden
	}

	return projectDetailFromModel(project), nil
}

func (s *DashboardService) UpdateProject(projectID uuid.UUID, userID uuid.UUID, req dto.UpdateProjectRequest) (*dto.ProjectDetail, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := strings.TrimSpace(req.SourceContent)
	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if title == "" || sourceContent == "" || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		if project.UserID != userID {
			return ErrForbidden
		}

		project.Title = title
		project.SourceContent = sourceContent
		project.Status = models.ProjectStatusReady
		if err := tx.Save(&project).Error; err != nil {
			return err
		}

		var existing []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&existing).Error; err != nil {
			return err
		}

		selected := make(map[string]struct{}, len(platforms))
		for _, platform := range platforms {
			selected[platform] = struct{}{}
		}

		for _, publication := range existing {
			if _, ok := selected[publication.Platform]; !ok {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"enabled":       false,
					"error_message": "",
					"status":        models.PublicationStatusDisabled,
				}).Error; err != nil {
					return err
				}
				continue
			}

			config, adaptedContent, status, err := buildPublicationPayload(&project, publication.Platform, title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			if err := tx.Model(&publication).Updates(map[string]interface{}{
				"adapted_content": adaptedContent,
				"config":          config,
				"enabled":         true,
				"error_message":   "",
				"last_attempt_at": nil,
				"published_at":    nil,
				"publish_url":     "",
				"remote_id":       "",
				"retry_count":     0,
				"status":          status,
			}).Error; err != nil {
				return err
			}
			delete(selected, publication.Platform)
		}

		for _, platform := range platforms {
			if _, ok := selected[platform]; !ok {
				continue
			}

			config, adaptedContent, status, err := buildPublicationPayload(&project, platform, title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return s.GetProject(projectID, &userID)
}

func buildPublicationPayload(project *models.Project, platform, title, summary, coverImageURL string) (datatypes.JSON, datatypes.JSON, string, error) {
	config, err := defaultPublicationConfig(title, summary, coverImageURL)
	if err != nil {
		return nil, nil, "", err
	}

	adaptedContent := datatypes.JSON([]byte(`{}`))
	status := models.PublicationStatusPending
	if p, err := publisher.Factory.GetPublisher(platform); err == nil {
		adapted, err := p.AdaptContent(project)
		if err != nil {
			return nil, nil, "", err
		}
		if len(adapted) > 0 {
			adaptedContent = datatypes.JSON(adapted)
		}
		status = models.PublicationStatusAdapted
	}

	return config, adaptedContent, status, nil
}

func projectDetailFromModel(project models.Project) *dto.ProjectDetail {
	publications := make([]dto.PublicationSummary, 0, len(project.Publications))
	for _, pub := range project.Publications {
		publications = append(publications, dto.PublicationSummary{
			ID:         pub.ID,
			Platform:   pub.Platform,
			Enabled:    pub.Enabled,
			Status:     pub.Status,
			PublishURL: pub.PublishURL,
		})
	}
	if publications == nil {
		publications = []dto.PublicationSummary{}
	}

	return &dto.ProjectDetail{
		ID:            project.ID,
		UserID:        project.UserID,
		Title:         project.Title,
		SourceContent: project.SourceContent,
		Status:        project.Status,
		CreatedAt:     project.CreatedAt,
		UpdatedAt:     project.UpdatedAt,
		Publications:  publications,
	}
}

func normalizeProjectPlatforms(input []string) ([]string, error) {
	seen := map[string]struct{}{}
	platforms := make([]string, 0, len(input))

	for _, raw := range input {
		platform := strings.TrimSpace(raw)
		if platform == "" {
			continue
		}
		if _, ok := allowedProjectPlatforms[platform]; !ok {
			return nil, ErrInvalidProject
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		platforms = append(platforms, platform)
	}

	return platforms, nil
}

func defaultPublicationConfig(title, summary, coverImageURL string) (datatypes.JSON, error) {
	digest := strings.TrimSpace(summary)
	if digest == "" {
		digest = title
	}
	config := map[string]interface{}{
		"digest": truncateRunes(digest, 120),
		"title":  title,
	}
	if coverImageURL := strings.TrimSpace(coverImageURL); coverImageURL != "" {
		config["cover_image_url"] = coverImageURL
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
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
	allowedKeys := []string{"title", "tags", "cover_image", "topics", "category", "original_declaration", "username"}
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
