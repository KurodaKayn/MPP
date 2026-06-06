package publish

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
)

type queueTestPublisher struct{}

func (p queueTestPublisher) ValidateConfig(_ []byte) error {
	return nil
}

func (p queueTestPublisher) Publish(_ context.Context, _ *models.ProjectPlatformPublication, _ *models.PlatformAccount) (string, string, error) {
	return "remote-id", "https://example.com/published", nil
}

type failingQueueTestPublisher struct{}

func (p failingQueueTestPublisher) ValidateConfig(_ []byte) error {
	return nil
}

func (p failingQueueTestPublisher) Publish(_ context.Context, _ *models.ProjectPlatformPublication, _ *models.PlatformAccount) (string, string, error) {
	return "", "", errors.New("platform unavailable")
}

type testPublishQueue struct {
	jobs            []PublishJob
	locks           map[string]string
	refreshes       int
	onAcquire       func(key, _ string)
	enqueueErr      error
	onAcquireLocked func(key, _ string)
}

func newTestPublishQueue() *testPublishQueue {
	return &testPublishQueue{locks: map[string]string{}}
}

func (q *testPublishQueue) Enqueue(_ context.Context, job PublishJob) error {
	if q.enqueueErr != nil {
		return q.enqueueErr
	}
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *testPublishQueue) Start(ctx context.Context, handler PublishJobHandler) {
	for len(q.jobs) > 0 {
		job := q.jobs[0]
		q.jobs = q.jobs[1:]
		_ = handler(ctx, job)
	}
}

func (q *testPublishQueue) AcquireLock(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	if _, exists := q.locks[key]; exists {
		if q.onAcquireLocked != nil {
			q.onAcquireLocked(key, value)
		}
		return false, nil
	}
	q.locks[key] = value
	if q.onAcquire != nil {
		q.onAcquire(key, value)
	}
	return true, nil
}

func (q *testPublishQueue) LockValue(_ context.Context, key string) (string, error) {
	return q.locks[key], nil
}

func (q *testPublishQueue) RefreshLock(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	if q.locks[key] != value {
		return false, nil
	}
	q.refreshes++
	return true, nil
}

func (q *testPublishQueue) ReleaseLock(_ context.Context, key, value string) error {
	if q.locks[key] == value {
		delete(q.locks, key)
	}
	return nil
}

type errorReportingPublishQueue struct {
	*testPublishQueue
	err error
}

func (q *errorReportingPublishQueue) Run(context.Context, PublishJobHandler) error {
	return q.err
}

func setupPublishQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		email TEXT NOT NULL,
		is_email_verified BOOLEAN NOT NULL DEFAULT 0,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		workspace_id TEXT,
		collab_document_id TEXT UNIQUE,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE project_collaborators (
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at DATETIME,
		PRIMARY KEY (project_id, user_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE project_activities (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		actor_user_id TEXT NOT NULL,
		target_user_id TEXT,
		event_type TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE platform_accounts (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		username TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'untested',
		credentials TEXT NOT NULL DEFAULT '{}',
		metadata TEXT NOT NULL DEFAULT '{}',
		cookies TEXT NOT NULL DEFAULT '[]',
		config TEXT NOT NULL DEFAULT '{}',
		avatar_url TEXT,
		last_tested_at DATETIME,
		last_test_error TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE project_platform_publications (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		status TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		adapted_content TEXT NOT NULL DEFAULT '{}',
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		last_attempt_at DATETIME,
		published_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE publish_events (
		id TEXT PRIMARY KEY,
		publication_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		job_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		event_type TEXT NOT NULL,
		status TEXT NOT NULL,
		message TEXT,
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME
	)`).Error)

	return db
}

func newPublishTestService(db *gorm.DB) *Service {
	return NewService(db, platformaccount.NewService(db))
}

func TestEnqueuePublishProjectQueuesAndLocksPublication(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-1"})
	require.NoError(t, err)
	require.Equal(t, models.PublicationStatusQueued, resp["status"])
	require.Len(t, queue.jobs, 1)
	require.Equal(t, uuid.Nil, queue.jobs[0].BrowserSessionID)

	lockKey := publishLockKey(project.ID, "wechat")
	require.Equal(t, queue.jobs[0].JobID.String(), queue.locks[lockKey])

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusQueued, saved.Status)
	require.NotNil(t, saved.LastAttemptAt)

	duplicate, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-1"})
	require.NoError(t, err)
	require.Equal(t, resp["job_id"], duplicate["job_id"])
}

func TestEnqueuePublishProjectRejectsProjectEditor(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &editor.ID, PublishRequest{IdempotencyKey: "click-editor"})

	require.ErrorIs(t, err, ErrForbidden)
	require.Nil(t, resp)
	require.Empty(t, queue.jobs)
}

func TestEnqueuePublishProjectReplaysDuplicateWhenLockWinsBeforeQueuedEvent(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}
	require.NoError(t, db.Create(&publication).Error)

	originalJobID := uuid.New()
	lockKey := publishLockKey(project.ID, "wechat")
	queue.locks[lockKey] = originalJobID.String()
	queue.onAcquireLocked = func(key, _ string) {
		if key != lockKey {
			return
		}
		require.NoError(t, db.Create(&models.PublishEvent{
			PublicationID:  publication.ID,
			ProjectID:      project.ID,
			UserID:         user.ID,
			Platform:       "wechat",
			JobID:          originalJobID,
			IdempotencyKey: "click-race",
			EventType:      "queued",
			Status:         models.PublicationStatusQueued,
		}).Error)
	}

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-race"})

	require.NoError(t, err)
	require.Equal(t, models.PublicationStatusQueued, resp["status"])
	require.Equal(t, originalJobID.String(), resp["job_id"])
	require.Empty(t, queue.jobs)
}

func TestEnqueuePublishProjectReplaysOriginalJobEventsAfterPublicationChanges(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	service.queue = newTestPublishQueue()

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusSucceeded,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
		RemoteID:       "newer-remote",
		PublishURL:     "https://example.com/newer",
	}
	require.NoError(t, db.Create(&publication).Error)

	jobID := uuid.New()
	queuedAt := time.Now().UTC().Add(-time.Minute)
	succeededAt := queuedAt.Add(time.Second)
	require.NoError(t, db.Create(&models.PublishEvent{
		PublicationID:  publication.ID,
		ProjectID:      project.ID,
		UserID:         user.ID,
		Platform:       "wechat",
		JobID:          jobID,
		IdempotencyKey: "click-original",
		EventType:      "queued",
		Status:         models.PublicationStatusQueued,
		CreatedAt:      queuedAt,
	}).Error)
	require.NoError(t, db.Create(&models.PublishEvent{
		PublicationID:  publication.ID,
		ProjectID:      project.ID,
		UserID:         user.ID,
		Platform:       "wechat",
		JobID:          jobID,
		IdempotencyKey: "click-original",
		EventType:      "succeeded",
		Status:         models.PublicationStatusSucceeded,
		RemoteID:       "event-remote",
		PublishURL:     "https://example.com/original",
		CreatedAt:      succeededAt,
	}).Error)
	require.NoError(t, db.Model(&publication).Updates(map[string]any{
		"enabled":       false,
		"error_message": "publication was later cancelled",
		"publish_url":   "https://example.com/changed",
		"remote_id":     "changed-remote",
		"status":        models.PublicationStatusCancelled,
	}).Error)

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-original"})

	require.NoError(t, err)
	require.Equal(t, models.PublicationStatusSucceeded, resp["status"])
	require.Equal(t, jobID.String(), resp["job_id"])
	require.Equal(t, "event-remote", resp["remote_id"])
	require.Equal(t, "https://example.com/original", resp["publish_url"])
	require.Empty(t, resp["error_message"])
}

func TestEnqueuePublishProjectDoesNotReplayFailedEnqueueAsQueued(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	queue.enqueueErr = errors.New("redis unavailable")
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	_, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-3"})
	require.Error(t, err)
	require.Empty(t, queue.jobs)

	var queuedEvents int64
	require.NoError(t, db.Model(&models.PublishEvent{}).
		Where("idempotency_key = ? AND event_type = ?", "click-3", "queued").
		Count(&queuedEvents).Error)
	require.Zero(t, queuedEvents)

	queue.enqueueErr = nil
	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-3"})
	require.NoError(t, err)
	require.Equal(t, models.PublicationStatusQueued, resp["status"])
	require.Len(t, queue.jobs, 1)
}

func TestEnqueuePublishProjectRejectsActivePublishingWithoutRedisLock(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	service.queue = newTestPublishQueue()

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	lastAttemptAt := time.Now().UTC()
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
		LastAttemptAt:  &lastAttemptAt,
	}).Error)

	_, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-2"})

	require.ErrorIs(t, err, ErrPublicationAlreadyPublishing)
}

func TestEnqueuePublishProjectRequiresPrepublishSyncForSyncingPublication(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusSyncing,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}
	require.NoError(t, db.Create(&publication).Error)

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-syncing"})

	require.ErrorIs(t, err, ErrPublicationRequiresSync)
	require.Nil(t, resp)
	require.Empty(t, queue.jobs)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", publication.ID).Error)
	require.Equal(t, models.PublicationStatusSyncing, saved.Status)
	require.Nil(t, saved.LastAttemptAt)
}

func TestEnqueuePublishProjectRejectsPublicationChangedToSyncingAfterLock(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}
	require.NoError(t, db.Create(&publication).Error)
	queue.onAcquire = func(_, _ string) {
		require.NoError(t, db.Model(&models.ProjectPlatformPublication{}).
			Where("id = ?", publication.ID).
			Update("status", models.PublicationStatusSyncing).Error)
	}

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID, PublishRequest{IdempotencyKey: "click-race"})

	require.ErrorIs(t, err, ErrPublicationRequiresSync)
	require.Nil(t, resp)
	require.Empty(t, queue.jobs)
	require.Empty(t, queue.locks)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", publication.ID).Error)
	require.Equal(t, models.PublicationStatusSyncing, saved.Status)
	require.Nil(t, saved.LastAttemptAt)
}

func TestProcessPublishJobPublishesAndReleasesLock(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	job := PublishJob{
		JobID:      uuid.New(),
		ProjectID:  project.ID,
		UserID:     user.ID,
		Platform:   "wechat",
		EnqueuedAt: time.Now().UTC(),
	}
	lockKey := publishLockKey(project.ID, "wechat")
	queue.locks[lockKey] = job.JobID.String()

	require.NoError(t, service.processPublishJob(context.Background(), job))

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusSucceeded, saved.Status)
	require.Equal(t, "remote-id", saved.RemoteID)
	require.Equal(t, "https://example.com/published", saved.PublishURL)
	require.Empty(t, queue.locks[lockKey])
}

func TestProcessPublishJobReacquiresExpiredLock(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", queueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	job := PublishJob{
		JobID:      uuid.New(),
		ProjectID:  project.ID,
		UserID:     user.ID,
		Platform:   "wechat",
		EnqueuedAt: time.Now().UTC(),
	}

	require.NoError(t, service.processPublishJob(context.Background(), job))

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusSucceeded, saved.Status)
	require.Empty(t, queue.locks[publishLockKey(project.ID, "wechat")])
}

func TestProcessPublishJobReturnsErrorForFailedPublication(t *testing.T) {
	db := setupPublishQueueTestDB(t)
	service := newPublishTestService(db)
	queue := newTestPublishQueue()
	service.queue = queue

	publisher.Factory.Register("wechat", failingQueueTestPublisher{})
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Queued post",
		SourceContent: "<p>ready</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         datatypes.JSON(`{"title":"Queued post"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	job := PublishJob{
		JobID:      uuid.New(),
		ProjectID:  project.ID,
		UserID:     user.ID,
		Platform:   "wechat",
		EnqueuedAt: time.Now().UTC(),
	}
	lockKey := publishLockKey(project.ID, "wechat")
	queue.locks[lockKey] = job.JobID.String()

	err := service.processPublishJob(context.Background(), job)

	require.Error(t, err)
	require.Empty(t, queue.locks[lockKey])

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusFailed, saved.Status)
	require.Equal(t, 1, saved.RetryCount)
	require.Contains(t, saved.ErrorMessage, "platform unavailable")
}

func TestRedisPublishQueueEnqueuesAsynqTask(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	queue := NewRedisPublishQueue(client)
	job := PublishJob{
		JobID:      uuid.New(),
		ProjectID:  uuid.New(),
		UserID:     uuid.New(),
		Platform:   "wechat",
		EnqueuedAt: time.Now().UTC(),
	}

	require.NoError(t, queue.Enqueue(context.Background(), job))

	inspector := asynq.NewInspectorFromRedisClient(client)
	tasks, err := inspector.ListPendingTasks(publishQueueName)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, publishTaskType, tasks[0].Type)
	require.Equal(t, publishTaskMaxRetry, tasks[0].MaxRetry)
	require.Equal(t, publishTaskTimeout, tasks[0].Timeout)

	var payload PublishJob
	require.NoError(t, json.Unmarshal(tasks[0].Payload, &payload))
	require.Equal(t, job, payload)
}

func TestStartPublishWorkerWithErrorsReportsRunnerFailure(t *testing.T) {
	expectedErr := errors.New("publish worker stopped")
	service := NewService(nil, nil)
	service.queue = &errorReportingPublishQueue{
		testPublishQueue: newTestPublishQueue(),
		err:              expectedErr,
	}

	errs := service.StartPublishWorkerWithErrors(context.Background())
	if errs == nil {
		t.Fatal("expected publish worker error channel")
	}

	select {
	case err := <-errs:
		require.ErrorIs(t, err, expectedErr)
	case <-time.After(time.Second):
		t.Fatal("expected publish worker error")
	}
}
