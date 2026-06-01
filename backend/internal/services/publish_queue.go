package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	publishQueueName         = "mpp:publish:jobs"
	publishLockKeyPrefix     = "mpp:publish:lock:"
	publishLockTTL           = 30 * time.Minute
	publishLockRefreshEvery  = publishLockTTL / 3
	publishQueueBlockTimeout = 5 * time.Second
	publishStaleAfter        = 2 * publishLockTTL
)

var (
	ErrPublicationAlreadyPublishing = errors.New("publication is already publishing")
	ErrPublishQueueEmpty            = errors.New("publish queue empty")
)

type PublishJob struct {
	JobID            uuid.UUID `json:"job_id"`
	ProjectID        uuid.UUID `json:"project_id"`
	UserID           uuid.UUID `json:"user_id"`
	Platform         string    `json:"platform"`
	BrowserSessionID uuid.UUID `json:"browser_session_id,omitempty"`
	EnqueuedAt       time.Time `json:"enqueued_at"`
}

type PublishQueue interface {
	Enqueue(ctx context.Context, job PublishJob) error
	Dequeue(ctx context.Context) (PublishJob, error)
	AcquireLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	LockValue(ctx context.Context, key string) (string, error)
	RefreshLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key, value string) error
}

type RedisPublishQueue struct {
	client    *redis.Client
	queueName string
}

func NewRedisPublishQueue(client *redis.Client) *RedisPublishQueue {
	return &RedisPublishQueue{
		client:    client,
		queueName: publishQueueName,
	}
}

func (q *RedisPublishQueue) Enqueue(ctx context.Context, job PublishJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, q.queueName, payload).Err()
}

func (q *RedisPublishQueue) Dequeue(ctx context.Context) (PublishJob, error) {
	result, err := q.client.BLPop(ctx, publishQueueBlockTimeout, q.queueName).Result()
	if errors.Is(err, redis.Nil) {
		return PublishJob{}, ErrPublishQueueEmpty
	}
	if err != nil {
		return PublishJob{}, err
	}
	if len(result) != 2 {
		return PublishJob{}, fmt.Errorf("unexpected redis queue response")
	}

	var job PublishJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return PublishJob{}, err
	}
	return job, nil
}

func (q *RedisPublishQueue) AcquireLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return q.client.SetNX(ctx, key, value, ttl).Result()
}

func (q *RedisPublishQueue) LockValue(ctx context.Context, key string) (string, error) {
	value, err := q.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return value, err
}

func (q *RedisPublishQueue) RefreshLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	const refreshLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`
	result, err := q.client.Eval(ctx, refreshLockScript, []string{key}, value, ttl.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (q *RedisPublishQueue) ReleaseLock(ctx context.Context, key, value string) error {
	const releaseLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	return q.client.Eval(ctx, releaseLockScript, []string{key}, value).Err()
}

func (s *DashboardService) EnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platform string, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	if s.publishQueue == nil {
		return s.PublishProject(projectID, platform, scopeUserID, uuid.Nil)
	}
	if scopeUserID == nil {
		return nil, ErrForbidden
	}

	project, pub, err := s.preparePublishJob(projectID, platform, *scopeUserID)
	if err != nil {
		return nil, err
	}

	// Create a browser session if needed for headless platforms
	var browserSessionID uuid.UUID
	var streamURL string
	if (platform == "douyin" || platform == "zhihu") && s.browserWorkerClient != nil {
		fmt.Printf("Creating visible browser session for %s publishing...\n", platform)

		// CLEANUP: Expire any existing active sessions for this user/platform to avoid unique constraint violation
		// We do this in a transaction or ensure it's done before the next create.
		if err := s.db.Model(&models.RemoteBrowserSession{}).
			Where("user_id = ? AND platform = ? AND status IN ?", *scopeUserID, platform, []string{"pending", "ready", "login_detected", "capturing"}).
			Updates(map[string]interface{}{
				"status":      models.BrowserSessionStatusExpired,
				"completed_at": time.Now(),
			}).Error; err != nil {
			fmt.Printf("Warning: failed to cleanup old sessions: %v\n", err)
		}

		// Generate stream token
		token, tokenHash, err := browsersession.GenerateStreamToken()
		if err != nil {
			fmt.Printf("Warning: failed to generate stream token: %v\n", err)
		}

		req := publisher.StartWorkerSessionRequest{
			SessionID:  uuid.New(),
			UserID:     *scopeUserID,
			Platform:   platform,
			LoginURL:   "about:blank",
			TTLSeconds: 600,
		}
		req.Viewport.Width = 1280
		req.Viewport.Height = 800

		resp, err := s.browserWorkerClient.CreateSession(ctx, req)
		if err == nil {
			browserSessionID = req.SessionID
			expiresAt := time.Now().Add(10 * time.Minute)
			tokenExpiresAt := browsersession.StreamTokenExpiresAt(expiresAt)
			streamURL = browsersession.BrowserSessionStreamURL(browserSessionID, token)

			session := &models.RemoteBrowserSession{
				ID:                    browserSessionID,
				UserID:                *scopeUserID,
				Platform:              platform,
				Status:                models.BrowserSessionStatusReady,
				WorkerSessionRef:      resp.WorkerSessionRef,
				ContainerID:           resp.ContainerID,
				CDPEndpointRef:        resp.CDPEndpointRef,
				StreamEndpointRef:     resp.StreamEndpointRef,
				ConnectTokenHash:      tokenHash,
				ConnectTokenExpiresAt: tokenExpiresAt,
				CreatedAt:             time.Now(),
				ExpiresAt:             expiresAt,
			}
			if err := s.db.Create(session).Error; err != nil {
				fmt.Printf("Error: failed to create session in DB: %v\n", err)
				return nil, fmt.Errorf("failed to initialize browser session: %w", err)
			}
		} else {
			fmt.Printf("Error: worker failed to create session: %v\n", err)
			return nil, fmt.Errorf("failed to start browser worker: %w", err)
		}
	}

	job := PublishJob{
		JobID:            uuid.New(),
		ProjectID:        project.ID,
		UserID:           *scopeUserID,
		Platform:         platform,
		BrowserSessionID: browserSessionID,
		EnqueuedAt:       time.Now().UTC(),
	}
	lockKey := publishLockKey(project.ID, platform)
	acquired, err := s.publishQueue.AcquireLock(ctx, lockKey, job.JobID.String(), publishLockTTL)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrPublicationAlreadyPublishing
	}

	if err := s.markPublicationQueued(&pub, job.EnqueuedAt); err != nil {
		_ = s.publishQueue.ReleaseLock(ctx, lockKey, job.JobID.String())
		return nil, err
	}
	if err := s.publishQueue.Enqueue(ctx, job); err != nil {
		_ = s.publishQueue.ReleaseLock(ctx, lockKey, job.JobID.String())
		_ = s.markPublicationFailed(project.ID, platform, "failed to enqueue publish job: "+err.Error())
		return nil, err
	}

	return map[string]interface{}{
		"status":             models.PublicationStatusPublishing,
		"job_id":             job.JobID.String(),
		"platform":           platform,
		"queued_at":          job.EnqueuedAt,
		"publish_url":        pub.PublishURL,
		"browser_session_id": browserSessionID,
		"stream_url":         streamURL,
	}, nil
}

func (s *DashboardService) BatchEnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
	results := make(map[string]map[string]interface{})
	for _, platform := range platforms {
		resp, err := s.EnqueuePublishProject(ctx, projectID, platform, scopeUserID)
		if err != nil {
			results[platform] = map[string]interface{}{"status": "error", "message": err.Error()}
			continue
		}
		results[platform] = resp
	}
	return results, nil
}

func (s *DashboardService) StartPublishWorker(ctx context.Context) {
	if s.publishQueue == nil {
		return
	}

	go s.runPublishWorker(ctx)
}

func (s *DashboardService) runPublishWorker(ctx context.Context) {
	for {
		job, err := s.publishQueue.Dequeue(ctx)
		if errors.Is(err, ErrPublishQueueEmpty) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("publish queue dequeue failed: %v", err)
			time.Sleep(time.Second)
			continue
		}

		s.processPublishJob(ctx, job)
	}
}

func (s *DashboardService) processPublishJob(ctx context.Context, job PublishJob) {
	if job.JobID == uuid.Nil || job.ProjectID == uuid.Nil || job.UserID == uuid.Nil || strings.TrimSpace(job.Platform) == "" {
		log.Printf("discarding invalid publish job: %+v", job)
		return
	}

	lockKey := publishLockKey(job.ProjectID, job.Platform)
	locked, err := s.ensurePublishJobLock(ctx, job, lockKey)
	if err != nil {
		log.Printf("publish lock check failed for job %s: %v", job.JobID, err)
		return
	}
	if !locked {
		log.Printf("skipping publish job %s because lock is not owned by this job", job.JobID)
		return
	}

	stopRefreshing := s.startPublishLockRefresh(ctx, lockKey, job.JobID.String())
	defer stopRefreshing()

	if _, err := s.PublishProject(job.ProjectID, job.Platform, &job.UserID, job.BrowserSessionID); err != nil {
		log.Printf("publish job %s failed: %v", job.JobID, err)
		if markErr := s.markPublicationFailed(job.ProjectID, job.Platform, err.Error()); markErr != nil {
			log.Printf("failed to mark publish job %s as failed: %v", job.JobID, markErr)
		}
	}

	if err := s.publishQueue.ReleaseLock(ctx, lockKey, job.JobID.String()); err != nil {
		log.Printf("publish lock release failed for job %s: %v", job.JobID, err)
	}
}

func (s *DashboardService) ensurePublishJobLock(ctx context.Context, job PublishJob, lockKey string) (bool, error) {
	lockValue, err := s.publishQueue.LockValue(ctx, lockKey)
	if err != nil {
		return false, err
	}
	if lockValue == job.JobID.String() {
		return true, nil
	}
	if lockValue != "" {
		return false, nil
	}

	stillPublishing, err := s.publicationStillPublishing(job.ProjectID, job.Platform)
	if err != nil {
		return false, err
	}
	if !stillPublishing {
		return false, nil
	}

	return s.publishQueue.AcquireLock(ctx, lockKey, job.JobID.String(), publishLockTTL)
}

func (s *DashboardService) startPublishLockRefresh(ctx context.Context, lockKey, lockValue string) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(publishLockRefreshEvery)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				refreshed, err := s.publishQueue.RefreshLock(ctx, lockKey, lockValue, publishLockTTL)
				if err != nil {
					log.Printf("publish lock refresh failed for %s: %v", lockKey, err)
					continue
				}
				if !refreshed {
					log.Printf("publish lock refresh skipped for %s because ownership changed", lockKey)
					return
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}

func (s *DashboardService) preparePublishJob(projectID uuid.UUID, platform string, userID uuid.UUID) (models.Project, models.ProjectPlatformPublication, error) {
	var project models.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return models.Project{}, models.ProjectPlatformPublication{}, ErrForbidden
	}

	var pub models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platform).First(&pub).Error; err != nil {
		return models.Project{}, models.ProjectPlatformPublication{}, fmt.Errorf("publication record not found for platform: %s", platform)
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusDisabled {
		return models.Project{}, models.ProjectPlatformPublication{}, ErrPublicationDisabled
	}
	if pub.Status == models.PublicationStatusPublishing && !publicationPublishingStale(pub) {
		return models.Project{}, models.ProjectPlatformPublication{}, ErrPublicationAlreadyPublishing
	}
	if _, err := publisher.Factory.GetPublisher(platform); err != nil {
		return models.Project{}, models.ProjectPlatformPublication{}, err
	}
	if pub.Status != models.PublicationStatusAdapted && pub.Status != models.PublicationStatusPublishing {
		p, err := publisher.Factory.GetPublisher(platform)
		if err != nil {
			return models.Project{}, models.ProjectPlatformPublication{}, err
		}
		if err := s.adaptPublicationForPublish(&project, &pub, p); err != nil {
			return models.Project{}, models.ProjectPlatformPublication{}, err
		}
	}

	return project, pub, nil
}

func (s *DashboardService) publicationStillPublishing(projectID uuid.UUID, platform string) (bool, error) {
	var pub models.ProjectPlatformPublication
	if err := s.db.Select("status").Where("project_id = ? AND platform = ?", projectID, platform).First(&pub).Error; err != nil {
		return false, err
	}
	return pub.Status == models.PublicationStatusPublishing, nil
}

func publicationPublishingStale(pub models.ProjectPlatformPublication) bool {
	if pub.Status != models.PublicationStatusPublishing || pub.LastAttemptAt == nil {
		return false
	}
	return time.Since(*pub.LastAttemptAt) > publishStaleAfter
}

func (s *DashboardService) markPublicationQueued(pub *models.ProjectPlatformPublication, queuedAt time.Time) error {
	return s.db.Model(pub).Updates(map[string]interface{}{
		"status":          models.PublicationStatusPublishing,
		"error_message":   "",
		"last_attempt_at": &queuedAt,
	}).Error
}

func (s *DashboardService) markPublicationFailed(projectID uuid.UUID, platform, message string) error {
	return s.db.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND platform = ?", projectID, platform).
		Updates(map[string]interface{}{
			"status":        models.PublicationStatusFailed,
			"error_message": sanitizeUserFacingErrorMessage(message),
			"retry_count":   gorm.Expr("retry_count + ?", 1),
		}).Error
}

func publishLockKey(projectID uuid.UUID, platform string) string {
	return publishLockKeyPrefix + projectID.String() + ":" + platform
}
