package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	publishTaskType         = "publish:project"
	publishQueueName        = "publish"
	publishTaskMaxRetry     = 3
	publishTaskTimeout      = 30 * time.Minute
	publishTaskRetention    = 24 * time.Hour
	publishWorkerConcurrent = 2
	publishLockKeyPrefix    = "mpp:publish:lock:"
	publishLockTTL          = 30 * time.Minute
	publishLockRefreshEvery = publishLockTTL / 3
	publishStaleAfter       = 2 * publishLockTTL
)

var (
	ErrPublicationAlreadyPublishing = errors.New("publication is already publishing")
	ErrPublishQueueEmpty            = errors.New("publish queue empty")
)

type PublishJob struct {
	JobID     uuid.UUID `json:"job_id"`
	ProjectID uuid.UUID `json:"project_id"`
	UserID    uuid.UUID `json:"user_id"`
	Platform  string    `json:"platform"`
	// Kept only so old Redis payloads still unmarshal; publishing never reuses live browser sessions.
	BrowserSessionID uuid.UUID `json:"browser_session_id,omitempty"`
	EnqueuedAt       time.Time `json:"enqueued_at"`
}

type PublishJobHandler func(context.Context, PublishJob) error

type PublishQueue interface {
	Enqueue(ctx context.Context, job PublishJob) error
	Start(ctx context.Context, handler PublishJobHandler)
	AcquireLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	LockValue(ctx context.Context, key string) (string, error)
	RefreshLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key, value string) error
}

type RedisPublishQueue struct {
	redisClient *redis.Client
	asynqClient *asynq.Client
}

func NewRedisPublishQueue(client *redis.Client) *RedisPublishQueue {
	return &RedisPublishQueue{
		redisClient: client,
		asynqClient: asynq.NewClientFromRedisClient(client),
	}
}

func (q *RedisPublishQueue) Enqueue(ctx context.Context, job PublishJob) error {
	task, err := newPublishTask(job)
	if err != nil {
		return err
	}
	_, err = q.asynqClient.EnqueueContext(
		ctx,
		task,
		asynq.Queue(publishQueueName),
		asynq.MaxRetry(publishTaskMaxRetry),
		asynq.Timeout(publishTaskTimeout),
		asynq.Retention(publishTaskRetention),
		asynq.TaskID(job.JobID.String()),
	)
	return err
}

func (q *RedisPublishQueue) Start(ctx context.Context, handler PublishJobHandler) {
	server := asynq.NewServerFromRedisClient(q.redisClient, asynq.Config{
		Concurrency: publishWorkerConcurrent,
		Queues: map[string]int{
			publishQueueName: 1,
		},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(publishTaskType, func(taskCtx context.Context, task *asynq.Task) error {
		job, err := publishJobFromTask(task)
		if err != nil {
			return fmt.Errorf("invalid publish task payload: %w: %w", err, asynq.SkipRetry)
		}
		return handler(taskCtx, job)
	})

	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()

	if err := server.Run(mux); err != nil && ctx.Err() == nil {
		log.Printf("publish worker stopped with error: %v", err)
	}
}

func newPublishTask(job PublishJob) (*asynq.Task, error) {
	payload, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(publishTaskType, payload), nil
}

func publishJobFromTask(task *asynq.Task) (PublishJob, error) {
	var job PublishJob
	if err := json.Unmarshal(task.Payload(), &job); err != nil {
		return PublishJob{}, err
	}
	return job, nil
}

func (q *RedisPublishQueue) AcquireLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return q.redisClient.SetNX(ctx, key, value, ttl).Result()
}

func (q *RedisPublishQueue) LockValue(ctx context.Context, key string) (string, error) {
	value, err := q.redisClient.Get(ctx, key).Result()
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
	result, err := q.redisClient.Eval(ctx, refreshLockScript, []string{key}, value, ttl.Milliseconds()).Int()
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
	return q.redisClient.Eval(ctx, releaseLockScript, []string{key}, value).Err()
}

func (s *Service) EnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platform string, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	if s.queue == nil {
		return s.PublishProject(projectID, platform, scopeUserID, uuid.Nil)
	}
	if scopeUserID == nil {
		return nil, ErrForbidden
	}

	project, pub, err := s.preparePublishJob(projectID, platform, *scopeUserID)
	if err != nil {
		return nil, err
	}

	job := PublishJob{
		JobID:      uuid.New(),
		ProjectID:  project.ID,
		UserID:     *scopeUserID,
		Platform:   platform,
		EnqueuedAt: time.Now().UTC(),
	}
	lockKey := publishLockKey(project.ID, platform)
	acquired, err := s.queue.AcquireLock(ctx, lockKey, job.JobID.String(), publishLockTTL)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrPublicationAlreadyPublishing
	}

	if err := s.markPublicationQueued(&pub, job.EnqueuedAt); err != nil {
		_ = s.queue.ReleaseLock(ctx, lockKey, job.JobID.String())
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		_ = s.queue.ReleaseLock(ctx, lockKey, job.JobID.String())
		_ = s.markPublicationFailed(project.ID, platform, "failed to enqueue publish job: "+err.Error())
		return nil, err
	}

	return map[string]interface{}{
		"status":      models.PublicationStatusPublishing,
		"job_id":      job.JobID.String(),
		"platform":    platform,
		"queued_at":   job.EnqueuedAt,
		"publish_url": pub.PublishURL,
	}, nil
}

func (s *Service) BatchEnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
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

func (s *Service) StartPublishWorker(ctx context.Context) {
	if s.queue == nil {
		return
	}

	go s.queue.Start(ctx, s.processPublishJob)
}

func (s *Service) processPublishJob(ctx context.Context, job PublishJob) error {
	if job.JobID == uuid.Nil || job.ProjectID == uuid.Nil || job.UserID == uuid.Nil || strings.TrimSpace(job.Platform) == "" {
		log.Printf("discarding invalid publish job: %+v", job)
		return nil
	}

	lockKey := publishLockKey(job.ProjectID, job.Platform)
	locked, err := s.ensurePublishJobLock(ctx, job, lockKey)
	if err != nil {
		log.Printf("publish lock check failed for job %s: %v", job.JobID, err)
		return err
	}
	if !locked {
		log.Printf("skipping publish job %s because lock is not owned by this job", job.JobID)
		return nil
	}

	stopRefreshing := s.startPublishLockRefresh(ctx, lockKey, job.JobID.String())
	defer stopRefreshing()

	resp, err := s.PublishProject(job.ProjectID, job.Platform, &job.UserID, uuid.Nil)
	if err != nil {
		log.Printf("publish job %s failed: %v", job.JobID, err)
		if markErr := s.markPublicationFailed(job.ProjectID, job.Platform, err.Error()); markErr != nil {
			log.Printf("failed to mark publish job %s as failed: %v", job.JobID, markErr)
		}
		if releaseErr := s.queue.ReleaseLock(context.Background(), lockKey, job.JobID.String()); releaseErr != nil {
			log.Printf("publish lock release failed for failed job %s: %v", job.JobID, releaseErr)
		}
		return err
	}
	if status, _ := resp["status"].(string); status == models.PublicationStatusFailed {
		message, _ := resp["error_message"].(string)
		err := fmt.Errorf("publish job %s failed: %s", job.JobID, message)
		if releaseErr := s.queue.ReleaseLock(context.Background(), lockKey, job.JobID.String()); releaseErr != nil {
			log.Printf("publish lock release failed for failed job %s: %v", job.JobID, releaseErr)
		}
		return err
	}

	if err := s.queue.ReleaseLock(ctx, lockKey, job.JobID.String()); err != nil {
		log.Printf("publish lock release failed for job %s: %v", job.JobID, err)
	}
	return nil
}

func (s *Service) ensurePublishJobLock(ctx context.Context, job PublishJob, lockKey string) (bool, error) {
	lockValue, err := s.queue.LockValue(ctx, lockKey)
	if err != nil {
		return false, err
	}
	if lockValue == job.JobID.String() {
		return true, nil
	}
	if lockValue != "" {
		return false, nil
	}

	retriable, err := s.publicationRetriableForJob(job.ProjectID, job.Platform)
	if err != nil {
		return false, err
	}
	if !retriable {
		return false, nil
	}

	return s.queue.AcquireLock(ctx, lockKey, job.JobID.String(), publishLockTTL)
}

func (s *Service) startPublishLockRefresh(ctx context.Context, lockKey, lockValue string) func() {
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
				refreshed, err := s.queue.RefreshLock(ctx, lockKey, lockValue, publishLockTTL)
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

func (s *Service) preparePublishJob(projectID uuid.UUID, platform string, userID uuid.UUID) (models.Project, models.ProjectPlatformPublication, error) {
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
		return models.Project{}, models.ProjectPlatformPublication{}, ErrPublicationRequiresSync
	}

	return project, pub, nil
}

func (s *Service) publicationRetriableForJob(projectID uuid.UUID, platform string) (bool, error) {
	var pub models.ProjectPlatformPublication
	if err := s.db.Select("enabled", "status").Where("project_id = ? AND platform = ?", projectID, platform).First(&pub).Error; err != nil {
		return false, err
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusDisabled {
		return false, nil
	}
	return pub.Status == models.PublicationStatusPublishing || pub.Status == models.PublicationStatusFailed, nil
}

func publicationPublishingStale(pub models.ProjectPlatformPublication) bool {
	if pub.Status != models.PublicationStatusPublishing || pub.LastAttemptAt == nil {
		return false
	}
	return time.Since(*pub.LastAttemptAt) > publishStaleAfter
}

func (s *Service) markPublicationQueued(pub *models.ProjectPlatformPublication, queuedAt time.Time) error {
	return s.db.Model(pub).Updates(map[string]interface{}{
		"status":          models.PublicationStatusPublishing,
		"error_message":   "",
		"last_attempt_at": &queuedAt,
	}).Error
}

func (s *Service) markPublicationFailed(projectID uuid.UUID, platform, message string) error {
	return s.db.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND platform = ?", projectID, platform).
		Updates(map[string]interface{}{
			"status":        models.PublicationStatusFailed,
			"error_message": SanitizeUserFacingErrorMessage(message),
			"retry_count":   gorm.Expr("retry_count + ?", 1),
		}).Error
}

func publishLockKey(projectID uuid.UUID, platform string) string {
	return publishLockKeyPrefix + projectID.String() + ":" + platform
}
