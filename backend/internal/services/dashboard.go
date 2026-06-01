package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (s *DashboardService) BatchPublishProject(projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
	results := make(map[string]map[string]interface{})
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, platform := range platforms {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			resp, err := s.PublishProject(projectID, p, scopeUserID, uuid.Nil)
			mu.Lock()
			if err != nil {
				results[p] = map[string]interface{}{"status": "error", "message": err.Error()}
			} else {
				results[p] = resp
			}
			mu.Unlock()
		}(platform)
	}

	wg.Wait()
	return results, nil
}

func (s *DashboardService) PublishProject(projectID uuid.UUID, platform string, scopeUserID *uuid.UUID, browserSessionID uuid.UUID) (map[string]interface{}, error) {
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

	// 3. Get Publisher from factory
	p, err := publisher.Factory.GetPublisher(platform)
	if err != nil {
		return nil, err
	}
	if pub.Status != models.PublicationStatusAdapted && pub.Status != models.PublicationStatusPublishing {
		if err := s.adaptPublicationForPublish(&proj, &pub, p); err != nil {
			return nil, err
		}
	}

	if err := s.applySavedWechatCredentialsToPublication(proj.UserID, &pub); err != nil {
		return nil, err
	}
	if err := s.applySavedXCredentialsToPublication(proj.UserID, &pub); err != nil {
		return nil, err
	}

	// 4. Get Platform Account (for Cookies/Session)
	var account models.PlatformAccount
	if err := s.db.Where("user_id = ? AND platform = ?", *scopeUserID, platform).First(&account).Error; err != nil {
		// Non-blocking
	}
	if err := s.applySavedBrowserCookies(context.Background(), proj.UserID, platform, &account); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if err := s.db.Model(&pub).Updates(map[string]interface{}{
		"status":          models.PublicationStatusPublishing,
		"error_message":   "",
		"last_attempt_at": &startedAt,
	}).Error; err != nil {
		return nil, err
	}

	// 5. Setup Remote Debugging if session provided
	ctx := context.Background()
	if browserSessionID != uuid.Nil {
		var session models.RemoteBrowserSession
		if err := s.db.First(&session, "id = ?", browserSessionID).Error; err == nil {
			// Connect using the predictable container name that Docker DNS can resolve
			remoteURL := fmt.Sprintf("http://mpp-session-%s:9222", session.ID.String())
			ctx = context.WithValue(ctx, publisher.ContextKeyRemoteURL, remoteURL)
		}
	}

	// 6. Execute Publish
	remoteID, publishURL, err := p.Publish(ctx, &pub, &account)

	// 7. Update Session Status if exists
	if browserSessionID != uuid.Nil {
		sessionStatus := models.BrowserSessionStatusExpired
		if err == nil {
			sessionStatus = models.BrowserSessionStatusExpired // Or a 'finished' status if you have one
		}
		s.db.Model(&models.RemoteBrowserSession{}).Where("id = ?", browserSessionID).Updates(map[string]interface{}{
			"status":       sessionStatus,
			"completed_at": time.Now(),
		})
	}

	// 8. Update DB
	status := models.PublicationStatusPublished
	errMsg := ""
	if err != nil {
		status = models.PublicationStatusFailed
		errMsg = sanitizeUserFacingErrorMessage(err.Error())
	}

	response := map[string]interface{}{
		"status":             status,
		"remote_id":          remoteID,
		"publish_url":        publishURL,
		"error_message":      errMsg,
		"browser_session_id": browserSessionID,
	}
	updates := map[string]interface{}{
		"status":        status,
		"remote_id":     remoteID,
		"publish_url":   publishURL,
		"error_message": errMsg,
	}
	if status == models.PublicationStatusPublished {
		publishedAt := time.Now().UTC()
		updates["published_at"] = &publishedAt
	} else {
		updates["retry_count"] = gorm.Expr("retry_count + ?", 1)
	}
	if err := s.db.Model(&pub).Updates(updates).Error; err != nil {
		return nil, err
	}

	return response, nil
}

func (s *DashboardService) CreateXPostIntent(projectID uuid.UUID, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	var proj models.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, *scopeUserID).First(&proj).Error; err != nil {
		return nil, ErrForbidden
	}

	var pub models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, "x").First(&pub).Error; err != nil {
		return nil, fmt.Errorf("publication record not found for platform: x")
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusDisabled {
		return nil, ErrPublicationDisabled
	}
	p, err := publisher.Factory.GetPublisher("x")
	if err != nil {
		return nil, err
	}
	if pub.Status != models.PublicationStatusAdapted {
		if err := s.adaptPublicationForPublish(&proj, &pub, p); err != nil {
			return nil, err
		}
	}

	publishURL, err := publisher.BuildXPostIntentURL(pub.AdaptedContent)
	if err != nil {
		return nil, err
	}

	if err := s.db.Model(&pub).Updates(map[string]interface{}{
		"publish_url":   publishURL,
		"error_message": "",
	}).Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status":      "manual_required",
		"platform":    "x",
		"publish_url": publishURL,
	}, nil
}

func (s *DashboardService) adaptPublicationForPublish(project *models.Project, pub *models.ProjectPlatformPublication, p publisher.PlatformPublisher) error {
	adaptedContent, err := p.AdaptContent(project)
	if err != nil {
		return err
	}

	content := pub.AdaptedContent
	if len(adaptedContent) > 0 {
		content = datatypes.JSON(adaptedContent)
	}

	updates := map[string]interface{}{
		"adapted_content": content,
		"error_message":   "",
		"last_attempt_at": nil,
		"published_at":    nil,
		"publish_url":     "",
		"remote_id":       "",
		"retry_count":     0,
		"status":          models.PublicationStatusAdapted,
	}
	if err := s.db.Model(pub).Updates(updates).Error; err != nil {
		return err
	}

	pub.AdaptedContent = content
	pub.ErrorMessage = ""
	pub.LastAttemptAt = nil
	pub.PublishedAt = nil
	pub.PublishURL = ""
	pub.RemoteID = ""
	pub.RetryCount = 0
	pub.Status = models.PublicationStatusAdapted
	return nil
}

func (s *DashboardService) applySavedBrowserCookies(ctx context.Context, userID uuid.UUID, platform string, account *models.PlatformAccount) error {
	if account == nil || !usesStoredBrowserCookies(platform) || account.UserID == uuid.Nil {
		return nil
	}

	cookies, err := publisher.NewCookieStore(s.db).Load(ctx, userID, platform)
	if err != nil {
		return fmt.Errorf("%w: %s cookies are unavailable: %v", ErrInvalidPlatformAccount, platform, err)
	}

	cookiesJSON, err := json.Marshal(cookies)
	if err != nil {
		return fmt.Errorf("failed to prepare %s cookies: %w", platform, err)
	}
	account.Cookies = datatypes.JSON(cookiesJSON)
	return nil
}

func usesStoredBrowserCookies(platform string) bool {
	switch platform {
	case "douyin", "zhihu":
		return true
	default:
		return false
	}
}

var ErrForbidden = errors.New("forbidden: you do not have permission to access this resource")
var ErrInvalidProject = errors.New("invalid project")
var ErrPublicationDisabled = errors.New("publication is disabled")
var ErrPublicationRequiresSync = errors.New("publication requires prepublish sync")
var ErrManualPublishUnsupported = errors.New("manual publish is only supported for x")

var sensitiveErrorQueryParamPattern = regexp.MustCompile(`(?i)(secret|access_token)=([^&"\s]+)`)
var allowedProjectPlatforms = map[string]struct{}{
	"douyin": {},
	"wechat": {},
	"x":      {},
	"zhihu":  {},
}

func sanitizeUserFacingErrorMessage(message string) string {
	return sensitiveErrorQueryParamPattern.ReplaceAllString(message, "$1=<redacted>")
}

type DashboardService struct {
	db                    *gorm.DB
	wechatTester          WechatConnectionTester
	xTester               XConnectionTester
	xOAuth2Provider       XOAuth2Provider
	xOAuth2States         XOAuth2StateStore
	publishQueue          PublishQueue
	browserWorkerClient   publisher.BrowserWorkerClient
	browserSessionService *browsersession.BrowserSessionService
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, WechatAPITester{}, XAPITester{})
}

func (s *DashboardService) SetBrowserWorkerClient(client publisher.BrowserWorkerClient) {
	s.browserWorkerClient = client
}

func (s *DashboardService) SetBrowserSessionService(svc *browsersession.BrowserSessionService) {
	s.browserSessionService = svc
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
	return &DashboardService{
		db:              db,
		wechatTester:    tester,
		xTester:         xTester,
		xOAuth2Provider: XOAuth2API{},
		xOAuth2States:   NewMemoryXOAuth2StateStore(),
	}
}

func NewDashboardServiceWithXOAuth2Provider(db *gorm.DB, provider XOAuth2Provider) *DashboardService {
	service := NewDashboardService(db)
	if provider != nil {
		service.xOAuth2Provider = provider
	}
	return service
}

func (s *DashboardService) SetPublishQueue(queue PublishQueue) {
	s.publishQueue = queue
}

func (s *DashboardService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.xOAuth2States = NewRedisXOAuth2StateStore(client)
	s.publishQueue = NewRedisPublishQueue(client)
	if s.browserSessionService != nil {
		s.browserSessionService.UseRedis(client)
	}
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
			config, adaptedContent, status, err := buildPendingPublicationPayload(title, req.Summary, req.CoverImageURL)
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

			config, err := defaultPublicationConfig(title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			if err := tx.Model(&publication).Updates(map[string]interface{}{
				"config":          config,
				"enabled":         true,
				"error_message":   "",
				"last_attempt_at": nil,
				"published_at":    nil,
				"publish_url":     "",
				"remote_id":       "",
				"retry_count":     0,
				"status":          models.PublicationStatusPending,
			}).Error; err != nil {
				return err
			}
			delete(selected, publication.Platform)
		}

		for _, platform := range platforms {
			if _, ok := selected[platform]; !ok {
				continue
			}

			config, adaptedContent, status, err := buildPendingPublicationPayload(title, req.Summary, req.CoverImageURL)
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

func (s *DashboardService) SaveProjectContent(projectID uuid.UUID, userID uuid.UUID, req dto.SaveProjectContentRequest) (*dto.ProjectDetail, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := strings.TrimSpace(req.SourceContent)
	if title == "" || sourceContent == "" {
		return nil, ErrInvalidProject
	}

	var project models.Project
	if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	if err := s.db.Model(&project).Updates(map[string]interface{}{
		"source_content": sourceContent,
		"status":         models.ProjectStatusReady,
		"title":          title,
	}).Error; err != nil {
		return nil, err
	}

	return s.GetProject(projectID, &userID)
}

func (s *DashboardService) SaveProjectPlatforms(projectID uuid.UUID, userID uuid.UUID, req dto.SaveProjectPlatformsRequest) (*dto.ProjectDetail, error) {
	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil || len(platforms) == 0 {
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

			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"enabled": true,
					"status":  models.PublicationStatusPending,
				}).Error; err != nil {
					return err
				}
			}
			delete(selected, publication.Platform)
		}

		for _, platform := range platforms {
			if _, ok := selected[platform]; !ok {
				continue
			}

			config, adaptedContent, status, err := buildPendingPublicationPayload(project.Title, "", "")
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

func buildPendingPublicationPayload(title, summary, coverImageURL string) (datatypes.JSON, datatypes.JSON, string, error) {
	config, err := defaultPublicationConfig(title, summary, coverImageURL)
	if err != nil {
		return nil, nil, "", err
	}

	return config, datatypes.JSON([]byte(`{}`)), models.PublicationStatusPending, nil
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

func (s *DashboardService) SyncProjectPrepublish(projectID uuid.UUID, userID uuid.UUID, req dto.SyncPrepublishRequest) (*dto.ProjectPublicationsResponse, error) {
	var project models.Project
	if err := s.db.Preload("Publications").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if len(platforms) == 0 {
		for _, publication := range project.Publications {
			if publication.Enabled && publication.Status != models.PublicationStatusDisabled {
				platforms = append(platforms, publication.Platform)
			}
		}
	}
	if len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			var publication models.ProjectPlatformPublication
			err := tx.Where("project_id = ? AND platform = ?", project.ID, platform).First(&publication).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				config, adaptedContent, status, err := buildPendingPublicationPayload(project.Title, "", "")
				if err != nil {
					return err
				}
				publication = models.ProjectPlatformPublication{
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
			} else if err != nil {
				return err
			}

			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"enabled": true,
					"status":  models.PublicationStatusPending,
				}).Error; err != nil {
					return err
				}
			}

			p, err := publisher.Factory.GetPublisher(platform)
			if err != nil {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"error_message": sanitizeUserFacingErrorMessage(err.Error()),
					"status":        models.PublicationStatusPending,
				}).Error; err != nil {
					return err
				}
				continue
			}

			adaptedContent, err := p.AdaptContent(&project)
			if err != nil {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"error_message": sanitizeUserFacingErrorMessage(err.Error()),
					"status":        models.PublicationStatusFailed,
				}).Error; err != nil {
					return err
				}
				continue
			}

			if err := tx.Model(&publication).Updates(map[string]interface{}{
				"adapted_content": datatypes.JSON(adaptedContent),
				"enabled":         true,
				"error_message":   "",
				"last_attempt_at": nil,
				"published_at":    nil,
				"publish_url":     "",
				"remote_id":       "",
				"retry_count":     0,
				"status":          models.PublicationStatusAdapted,
			}).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return s.GetProjectPublications(projectID, &userID, true)
}

func (s *DashboardService) UpdateProjectPrepublishDraft(projectID uuid.UUID, userID uuid.UUID, platform string, req dto.UpdatePrepublishDraftRequest) (*dto.ProjectPublicationsResponse, error) {
	var project models.Project
	if err := s.db.Select("id, user_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	platforms, err := normalizeProjectPlatforms([]string{platform})
	if err != nil || len(platforms) != 1 {
		return nil, ErrInvalidProject
	}
	if len(req.AdaptedContent) == 0 {
		return nil, ErrInvalidProject
	}

	adaptedContent, err := json.Marshal(req.AdaptedContent)
	if err != nil {
		return nil, err
	}

	var publication models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platforms[0]).First(&publication).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&publication).Updates(map[string]interface{}{
		"adapted_content": datatypes.JSON(adaptedContent),
		"enabled":         true,
		"error_message":   "",
		"last_attempt_at": nil,
		"published_at":    nil,
		"publish_url":     "",
		"remote_id":       "",
		"retry_count":     0,
		"status":          models.PublicationStatusAdapted,
	}).Error; err != nil {
		return nil, err
	}

	return s.GetProjectPublications(projectID, &userID, true)
}

func (s *DashboardService) GetProjectPublications(projectID uuid.UUID, scopeUserID *uuid.UUID, includeContent bool) (*dto.ProjectPublicationsResponse, error) {
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

		// Safe parse adapted content
		var rawContent map[string]interface{}
		_ = json.Unmarshal(pub.AdaptedContent, &rawContent)
		safeContent := rawContent
		if !includeContent {
			safeContent = summarizeAdaptedContent(rawContent)
		}

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
