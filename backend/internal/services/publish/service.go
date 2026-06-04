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
	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
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

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	if s.accounts != nil {
		scoped.accounts = s.accounts.WithContext(ctx)
	}
	return &scoped
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
		return nil, ErrPublicationRequiresSync
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

	var remoteID string
	var publishURL string
	publishPolicy := resilience.DefaultOperationPolicy("publish-" + platform)
	publishPolicy.MaxAttempts = 1
	err = resilience.Run(
		context.Background(),
		publishPolicy,
		func(ctx context.Context) error {
			var publishErr error
			remoteID, publishURL, publishErr = p.Publish(ctx, &pub, &account)
			return publishErr
		},
	)

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
	if pub.Status != models.PublicationStatusAdapted {
		return nil, ErrPublicationRequiresSync
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
