package publish

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type queueTestPublisher struct{}

func (p queueTestPublisher) ValidateConfig(config []byte) error {
	return nil
}

func (p queueTestPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	return []byte(`{"format":"html","html":"adapted"}`), nil
}

func (p queueTestPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	return "remote-id", "https://example.com/published", nil
}

type testPublishQueue struct {
	jobs      []PublishJob
	locks     map[string]string
	refreshes int
}

func newTestPublishQueue() *testPublishQueue {
	return &testPublishQueue{locks: map[string]string{}}
}

func (q *testPublishQueue) Enqueue(ctx context.Context, job PublishJob) error {
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *testPublishQueue) Dequeue(ctx context.Context) (PublishJob, error) {
	if len(q.jobs) == 0 {
		return PublishJob{}, ErrPublishQueueEmpty
	}
	job := q.jobs[0]
	q.jobs = q.jobs[1:]
	return job, nil
}

func (q *testPublishQueue) AcquireLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if _, exists := q.locks[key]; exists {
		return false, nil
	}
	q.locks[key] = value
	return true, nil
}

func (q *testPublishQueue) LockValue(ctx context.Context, key string) (string, error) {
	return q.locks[key], nil
}

func (q *testPublishQueue) RefreshLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if q.locks[key] != value {
		return false, nil
	}
	q.refreshes++
	return true, nil
}

func (q *testPublishQueue) ReleaseLock(ctx context.Context, key, value string) error {
	if q.locks[key] == value {
		delete(q.locks, key)
	}
	return nil
}

func setupPublishQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
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

	resp, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID)
	require.NoError(t, err)
	require.Equal(t, models.PublicationStatusPublishing, resp["status"])
	require.Len(t, queue.jobs, 1)

	lockKey := publishLockKey(project.ID, "wechat")
	require.Equal(t, queue.jobs[0].JobID.String(), queue.locks[lockKey])

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusPublishing, saved.Status)
	require.NotNil(t, saved.LastAttemptAt)

	_, err = service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID)
	require.True(t, errors.Is(err, ErrPublicationAlreadyPublishing))
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

	_, err := service.EnqueuePublishProject(context.Background(), project.ID, "wechat", &user.ID)

	require.True(t, errors.Is(err, ErrPublicationAlreadyPublishing))
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

	service.processPublishJob(context.Background(), job)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusPublished, saved.Status)
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

	service.processPublishJob(context.Background(), job)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusPublished, saved.Status)
	require.Empty(t, queue.locks[publishLockKey(project.ID, "wechat")])
}
