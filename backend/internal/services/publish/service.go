package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
)

var ErrForbidden = errors.New("forbidden: you do not have permission to access this resource")
var ErrPublicationDisabled = errors.New("publication is disabled")
var ErrPublicationRequiresSync = errors.New("publication requires prepublish sync")
var ErrManualPublishUnsupported = errors.New("manual publish is only supported for x")

var sensitiveErrorQueryParamPattern = regexp.MustCompile(`(?i)(secret|access_token|x-amz-credential|x-amz-signature|x-amz-security-token)=([^&"\s]+)`)

type Service struct {
	db                    *gorm.DB
	accounts              *platformaccount.Service
	queue                 PublishQueue
	publishJobObserver    PublishJobObserver
	browserWorkerClient   publisher.BrowserWorkerClient
	browserSessionService *browsersession.BrowserSessionService
	objectStorage         objectstorage.Client
	storageConfig         objectstorage.Config
	dashboardCache        DashboardCacheInvalidator
}

type PublishJobObserver interface {
	ObservePublishJob(platform string, result string)
}

type DashboardCacheInvalidator interface {
	InvalidateDashboardProjectListCache(ctx context.Context)
	InvalidateDashboardStatsCache(ctx context.Context)
}

const (
	publishJobResultSuccess = "success"
	publishJobResultError   = "error"
)

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
	if s.browserSessionService != nil {
		scoped.browserSessionService = s.browserSessionService.WithContext(ctx)
	}
	return &scoped
}

func (s *Service) SetQueue(queue PublishQueue) {
	s.queue = queue
}

func (s *Service) SetPublishJobObserver(observer PublishJobObserver) {
	s.publishJobObserver = observer
}

func (s *Service) SetBrowserWorkerClient(client publisher.BrowserWorkerClient) {
	s.browserWorkerClient = client
}

func (s *Service) SetBrowserSessionService(service *browsersession.BrowserSessionService) {
	s.browserSessionService = service
}

func (s *Service) UseObjectStorage(client objectstorage.Client, config objectstorage.Config) {
	s.objectStorage = client
	s.storageConfig = config
}

func (s *Service) SetDashboardCacheInvalidator(invalidator DashboardCacheInvalidator) {
	s.dashboardCache = invalidator
}

func (s *Service) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.queue = NewRedisPublishQueue(client)
}

func (s *Service) observePublishJob(platform string, result string) {
	if s.publishJobObserver != nil {
		s.publishJobObserver.ObservePublishJob(platform, result)
	}
}

func (s *Service) invalidateDashboardCaches(ctx context.Context) {
	if s.dashboardCache == nil {
		return
	}
	s.dashboardCache.InvalidateDashboardProjectListCache(ctx)
	s.dashboardCache.InvalidateDashboardStatsCache(ctx)
}

func (s *Service) invalidateDashboardProjectListCache(ctx context.Context) {
	if s.dashboardCache == nil {
		return
	}
	s.dashboardCache.InvalidateDashboardProjectListCache(ctx)
}

func SanitizeUserFacingErrorMessage(message string) string {
	return sensitiveErrorQueryParamPattern.ReplaceAllString(message, "$1=<redacted>")
}

func (s *Service) BatchPublishProject(projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]any, error) {
	results := make(map[string]map[string]any)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, platform := range platforms {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			resp, err := s.PublishProject(projectID, p, scopeUserID, uuid.Nil)
			mu.Lock()
			if err != nil {
				results[p] = map[string]any{"status": "error", "message": err.Error()}
			} else {
				results[p] = resp
			}
			mu.Unlock()
		}(platform)
	}

	wg.Wait()
	return results, nil
}

func (s *Service) PublishProject(projectID uuid.UUID, platform string, scopeUserID *uuid.UUID, scheduleID uuid.UUID) (map[string]any, error) {
	// Remote browser sessions are only for account connection and cookie capture.
	// Publish jobs must be durable across Redis workers, so they load saved credentials instead.
	ctx := context.Background()

	if scopeUserID == nil {
		return nil, ErrForbidden
	}
	proj, err := s.projectForPublish(ctx, projectID, *scopeUserID)
	if err != nil {
		return nil, err
	}

	var pub models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platform).First(&pub).Error; err != nil {
		return nil, fmt.Errorf("publication record not found for platform: %s", platform)
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusCancelled {
		return nil, ErrPublicationDisabled
	}

	p, err := publisher.Factory.GetPublisher(platform)
	if err != nil {
		return nil, err
	}
	if pub.Status == models.PublicationStatusSyncing || (!publicationHasSyncedDraft(pub) && pub.Status != models.PublicationStatusQueued && pub.Status != models.PublicationStatusPublishing) {
		return nil, ErrPublicationRequiresSync
	}

	startedAt := time.Now().UTC()
	attempt, err := s.startPublishAttempt(scheduleID, startedAt)
	if err != nil {
		return nil, err
	}
	failAttempt := func(err error) error {
		if attempt != nil {
			_ = s.finishPublishAttempt(attempt, models.PublishAttemptStatusFailed, "", "", SanitizeUserFacingErrorMessage(err.Error()))
		}
		return err
	}

	if err := s.accounts.ApplySavedCredentialsToPublication(*scopeUserID, &pub); err != nil {
		if errors.Is(err, platformaccount.ErrPlatformAccountForbidden) {
			return nil, failAttempt(ErrForbidden)
		}
		return nil, failAttempt(err)
	}
	pub, err = s.preparePublicationMediaRefs(ctx, proj, pub)
	if err != nil {
		return nil, failAttempt(err)
	}

	var account models.PlatformAccount
	if pub.PlatformAccountID != nil && *pub.PlatformAccountID != uuid.Nil {
		if err := s.db.Where("id = ?", *pub.PlatformAccountID).First(&account).Error; err != nil {
			return nil, err
		}
	}
	if usesStoredBrowserCookies(platform) {
		if account.ID == uuid.Nil {
			return nil, failAttempt(fmt.Errorf("%w: %s account is not connected", platformaccount.ErrInvalidPlatformAccount, platform))
		}
		if err := s.applySavedBrowserCookies(ctx, *scopeUserID, platform, &account); err != nil {
			return nil, failAttempt(err)
		}
	}

	if err := s.markPublicationPublishing(&pub, startedAt); err != nil {
		return nil, failAttempt(err)
	}
	s.invalidateDashboardCaches(ctx)

	var remoteID string
	var publishURL string
	publishPolicy := resilience.DefaultOperationPolicy("publish-" + platform)
	publishPolicy.MaxAttempts = 1
	err = resilience.Run(
		ctx,
		publishPolicy,
		func(ctx context.Context) error {
			var publishErr error
			remoteID, publishURL, publishErr = p.Publish(ctx, &pub, &account)
			return publishErr
		},
	)

	status := models.PublicationStatusSucceeded
	errMsg := ""
	if err != nil {
		status = models.PublicationStatusFailed
		errMsg = SanitizeUserFacingErrorMessage(err.Error())
	}

	response := map[string]any{
		"status":             status,
		"remote_id":          remoteID,
		"publish_url":        publishURL,
		"error_message":      errMsg,
		"browser_session_id": uuid.Nil,
	}
	updates := map[string]any{
		"status":        status,
		"remote_id":     remoteID,
		"publish_url":   publishURL,
		"error_message": errMsg,
	}
	if status == models.PublicationStatusSucceeded {
		publishedAt := time.Now().UTC()
		updates["published_at"] = &publishedAt
	} else {
		updates["retry_count"] = gorm.Expr("retry_count + ?", 1)
	}
	if err := s.db.Model(&pub).Updates(updates).Error; err != nil {
		return nil, failAttempt(err)
	}
	attemptStatus := models.PublishAttemptStatusSucceeded
	if status == models.PublicationStatusFailed {
		attemptStatus = models.PublishAttemptStatusFailed
	}
	if err := s.finishPublishAttempt(attempt, attemptStatus, remoteID, publishURL, errMsg); err != nil {
		return nil, err
	}
	s.invalidateDashboardCaches(ctx)
	if err := s.recordProjectPublishActivity(projectID, *scopeUserID, models.ProjectActivityPublishCompleted, map[string]any{
		"platform":  platform,
		"status":    status,
		"remote_id": remoteID,
	}); err != nil {
		log.Printf("failed to record project publish activity for project %s platform %s: %v", projectID, platform, err)
	}

	return response, nil
}

func (s *Service) markPublicationPublishing(pub *models.ProjectPlatformPublication, startedAt time.Time) error {
	result := s.db.Model(&models.ProjectPlatformPublication{}).
		Where("id = ? AND status = ?", pub.ID, pub.Status).
		Updates(map[string]any{
			"status":          models.PublicationStatusPublishing,
			"error_message":   "",
			"last_attempt_at": &startedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return s.publicationStateChangedError(pub.ID)
	}
	pub.Status = models.PublicationStatusPublishing
	pub.LastAttemptAt = &startedAt
	return nil
}

func (s *Service) CreateXPostIntent(projectID uuid.UUID, scopeUserID *uuid.UUID) (map[string]any, error) {
	if scopeUserID == nil {
		return nil, ErrForbidden
	}
	if _, err := s.projectForPublish(context.Background(), projectID, *scopeUserID); err != nil {
		return nil, err
	}

	var pub models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, "x").First(&pub).Error; err != nil {
		return nil, fmt.Errorf("publication record not found for platform: x")
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusCancelled {
		return nil, ErrPublicationDisabled
	}
	if !publicationHasSyncedDraft(pub) {
		return nil, ErrPublicationRequiresSync
	}

	publishURL, err := publisher.BuildXPostIntentURL(pub.AdaptedContent)
	if err != nil {
		return nil, err
	}

	if err := s.db.Model(&pub).Updates(map[string]any{
		"publish_url":   publishURL,
		"error_message": "",
	}).Error; err != nil {
		return nil, err
	}
	s.invalidateDashboardProjectListCache(context.Background())

	return map[string]any{
		"status":      "manual_required",
		"platform":    "x",
		"publish_url": publishURL,
	}, nil
}

func normalizeIdempotencyKey(key string) string {
	return strings.TrimSpace(key)
}

func (s *Service) publicationStateChangedError(publicationID uuid.UUID) error {
	var pub models.ProjectPlatformPublication
	if err := s.db.Select("status", "last_attempt_at").First(&pub, "id = ?", publicationID).Error; err != nil {
		return err
	}
	if pub.Status == models.PublicationStatusQueued || pub.Status == models.PublicationStatusPublishing {
		return ErrPublicationAlreadyPublishing
	}
	return ErrPublicationRequiresSync
}

func publicationHasSyncedDraft(pub models.ProjectPlatformPublication) bool {
	content := strings.TrimSpace(string(pub.AdaptedContent))
	return content != "" && content != "{}" && content != "null"
}

func (s *Service) recordPublishEvent(event models.PublishEvent) error {
	if event.ProjectID == uuid.Nil || event.UserID == uuid.Nil || event.Platform == "" || event.JobID == uuid.Nil {
		return nil
	}
	if event.Metadata == nil {
		event.Metadata = datatypes.JSON(`{}`)
	}
	if event.Status == "" {
		event.Status = models.PublicationStatusDraft
	}
	return s.db.Create(&event).Error
}

func (s *Service) recordProjectPublishActivity(projectID uuid.UUID, userID uuid.UUID, eventType string, metadata map[string]any) error {
	if projectID == uuid.Nil || userID == uuid.Nil || strings.TrimSpace(eventType) == "" {
		return nil
	}
	payload := datatypes.JSON([]byte(`{}`))
	if metadata != nil {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		payload = datatypes.JSON(encoded)
	}
	return s.db.Create(&models.ProjectActivity{
		ProjectID:   projectID,
		ActorUserID: userID,
		EventType:   eventType,
		Metadata:    payload,
	}).Error
}

func (s *Service) findIdempotentPublishResponse(projectID uuid.UUID, platform string, userID uuid.UUID, key string) (map[string]any, bool, error) {
	if strings.TrimSpace(key) == "" {
		return nil, false, nil
	}

	var queued models.PublishEvent
	err := s.db.
		Where("project_id = ? AND platform = ? AND user_id = ? AND idempotency_key = ? AND event_type = ?", projectID, platform, userID, key, "queued").
		Order("created_at DESC").
		First(&queued).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	event := queued
	var events []models.PublishEvent
	err = s.db.
		Where("project_id = ? AND platform = ? AND user_id = ? AND job_id = ?", projectID, platform, userID, queued.JobID).
		Order("created_at ASC").
		Find(&events).Error
	if err != nil {
		return nil, false, err
	}
	for _, candidate := range events {
		if publishEventNewerForReplay(candidate, event) {
			event = candidate
		}
	}

	resp := map[string]any{
		"status":          event.Status,
		"job_id":          queued.JobID.String(),
		"idempotency_key": key,
		"platform":        platform,
		"queued_at":       queued.CreatedAt,
		"remote_id":       event.RemoteID,
		"publish_url":     event.PublishURL,
		"error_message":   event.ErrorMessage,
	}
	return resp, true, nil
}

func publishEventNewerForReplay(candidate, current models.PublishEvent) bool {
	if candidate.CreatedAt.After(current.CreatedAt) {
		return true
	}
	if current.CreatedAt.After(candidate.CreatedAt) {
		return false
	}
	return publishEventReplayRank(candidate.EventType) > publishEventReplayRank(current.EventType)
}

func publishEventReplayRank(eventType string) int {
	switch eventType {
	case "succeeded", "failed":
		return 4
	case "started":
		return 3
	case "queued":
		return 2
	case "requested":
		return 1
	default:
		return 0
	}
}

func (s *Service) waitForIdempotentPublishResponse(ctx context.Context, projectID uuid.UUID, platform string, userID uuid.UUID, key string) (map[string]any, bool, error) {
	deadline := time.NewTimer(publishReplayWait)
	defer deadline.Stop()

	ticker := time.NewTicker(publishReplayPoll)
	defer ticker.Stop()

	for {
		resp, ok, err := s.findIdempotentPublishResponse(projectID, platform, userID, key)
		if err != nil || ok {
			return resp, ok, err
		}

		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-deadline.C:
			return nil, false, nil
		case <-ticker.C:
		}
	}
}

func (s *Service) applySavedBrowserCookies(ctx context.Context, userID uuid.UUID, platform string, account *models.PlatformAccount) error {
	if account == nil || !usesStoredBrowserCookies(platform) || account.UserID == uuid.Nil {
		return nil
	}

	cookies, err := publisher.NewCookieStore(s.db).LoadForAccount(ctx, userID, account.ID, platform)
	if err != nil {
		return fmt.Errorf("%w: %s cookies are unavailable: %w", platformaccount.ErrInvalidPlatformAccount, platform, err)
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
