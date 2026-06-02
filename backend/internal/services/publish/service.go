package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("forbidden: you do not have permission to access this resource")
var ErrPublicationDisabled = errors.New("publication is disabled")
var ErrPublicationRequiresSync = errors.New("publication requires prepublish sync")
var ErrManualPublishUnsupported = errors.New("manual publish is only supported for x")

var sensitiveErrorQueryParamPattern = regexp.MustCompile(`(?i)(secret|access_token)=([^&"\s]+)`)

type Service struct {
	db       *gorm.DB
	accounts *platformaccount.Service
	queue    PublishQueue
}

func NewService(db *gorm.DB, accounts *platformaccount.Service) *Service {
	if accounts == nil {
		accounts = platformaccount.NewService(db)
	}
	return &Service{
		db:       db,
		accounts: accounts,
	}
}

func (s *Service) SetQueue(queue PublishQueue) {
	s.queue = queue
}

func (s *Service) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.queue = NewRedisPublishQueue(client)
}

func SanitizeUserFacingErrorMessage(message string) string {
	return sensitiveErrorQueryParamPattern.ReplaceAllString(message, "$1=<redacted>")
}

func (s *Service) BatchPublishProject(projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
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

func (s *Service) PublishProject(projectID uuid.UUID, platform string, scopeUserID *uuid.UUID, browserSessionID uuid.UUID) (map[string]interface{}, error) {
	// Remote browser sessions are only for account connection and cookie capture.
	// Publish jobs must be durable across Redis workers, so they load saved credentials instead.
	browserSessionID = uuid.Nil

	var proj models.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, *scopeUserID).First(&proj).Error; err != nil {
		return nil, ErrForbidden
	}

	var pub models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platform).First(&pub).Error; err != nil {
		return nil, fmt.Errorf("publication record not found for platform: %s", platform)
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusDisabled {
		return nil, ErrPublicationDisabled
	}

	p, err := publisher.Factory.GetPublisher(platform)
	if err != nil {
		return nil, err
	}
	if pub.Status != models.PublicationStatusAdapted && pub.Status != models.PublicationStatusPublishing {
		if err := s.adaptPublicationForPublish(&proj, &pub, p); err != nil {
			return nil, err
		}
	}

	if err := s.accounts.ApplySavedCredentialsToPublication(proj.UserID, &pub); err != nil {
		return nil, err
	}

	var account models.PlatformAccount
	accountErr := s.db.Where("user_id = ? AND platform = ?", *scopeUserID, platform).First(&account).Error
	if accountErr != nil && !errors.Is(accountErr, gorm.ErrRecordNotFound) {
		return nil, accountErr
	}
	if usesStoredBrowserCookies(platform) {
		if errors.Is(accountErr, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s account is not connected", platformaccount.ErrInvalidPlatformAccount, platform)
		}
		if err := s.applySavedBrowserCookies(context.Background(), proj.UserID, platform, &account); err != nil {
			return nil, err
		}
	}

	startedAt := time.Now().UTC()
	if err := s.db.Model(&pub).Updates(map[string]interface{}{
		"status":          models.PublicationStatusPublishing,
		"error_message":   "",
		"last_attempt_at": &startedAt,
	}).Error; err != nil {
		return nil, err
	}

	remoteID, publishURL, err := p.Publish(context.Background(), &pub, &account)

	status := models.PublicationStatusPublished
	errMsg := ""
	if err != nil {
		status = models.PublicationStatusFailed
		errMsg = SanitizeUserFacingErrorMessage(err.Error())
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

func (s *Service) CreateXPostIntent(projectID uuid.UUID, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
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

func (s *Service) adaptPublicationForPublish(project *models.Project, pub *models.ProjectPlatformPublication, p publisher.PlatformPublisher) error {
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

func (s *Service) applySavedBrowserCookies(ctx context.Context, userID uuid.UUID, platform string, account *models.PlatformAccount) error {
	if account == nil || !usesStoredBrowserCookies(platform) || account.UserID == uuid.Nil {
		return nil
	}

	cookies, err := publisher.NewCookieStore(s.db).Load(ctx, userID, platform)
	if err != nil {
		return fmt.Errorf("%w: %s cookies are unavailable: %v", platformaccount.ErrInvalidPlatformAccount, platform, err)
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
