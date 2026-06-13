package stats_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestGetStats(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test1"}
	u2 := models.User{Username: "test2"}
	db.Create(&u1)
	db.Create(&u2)

	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c", Status: models.ProjectStatusDraft}
	p2 := models.Project{UserID: u2.ID, Title: "p2", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p1)
	db.Create(&p2)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished})
	db.Create(&models.ProjectPlatformPublication{ProjectID: p2.ID, Platform: "zhihu", Status: models.PublicationStatusFailed})

	// Test Admin scope (nil scopeUserID)
	stats, err := s.GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)

	// Test Personal scope (u1)
	statsScoped, errScoped := s.GetStats(&u1.ID)
	require.NoError(t, errScoped)
	assert.Equal(t, int64(1), statsScoped.TotalUsers)
	assert.Equal(t, int64(1), statsScoped.TotalProjects)
	assert.Equal(t, int64(1), statsScoped.TotalPublishedPublications)
	assert.Equal(t, int64(0), statsScoped.TotalFailedPublications)
}

func TestGetStatsUsesCompleteWorkspaceReadModel(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	userID := uuid.New()
	workspaceID := uuid.New()
	now := time.Now().UTC()

	require.NoError(t, db.Create(&models.User{ID: userID, Username: "stats-readmodel", Email: "stats-readmodel@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.Workspace{ID: workspaceID, OwnerUserID: userID, Name: "Stats read model", Status: models.WorkspaceStatusActive, CreatedAt: now, UpdatedAt: now}).Error)
	for i := range 7 {
		project := models.Project{
			ID:            uuid.New(),
			UserID:        userID,
			WorkspaceID:   &workspaceID,
			Title:         "Stats read model fact",
			SourceContent: "content",
			Status:        models.ProjectStatusReady,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		require.NoError(t, db.Create(&project).Error)
		publicationStatus := models.PublicationStatusPublished
		if i >= 5 {
			publicationStatus = models.PublicationStatusFailed
		}
		require.NoError(t, db.Create(&models.ProjectPlatformPublication{
			ProjectID: project.ID,
			Platform:  "wechat",
			Status:    publicationStatus,
		}).Error)
	}
	require.NoError(t, db.Create(&models.WorkspaceDashboardStats{
		WorkspaceID:                workspaceID,
		TotalProjects:              7,
		TotalPublishedPublications: 5,
		TotalFailedPublications:    2,
		TotalMembers:               3,
		RefreshedAt:                now,
	}).Error)

	stats, err := s.GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(7), stats.TotalProjects)
	assert.Equal(t, int64(5), stats.TotalPublishedPublications)
	assert.Equal(t, int64(2), stats.TotalFailedPublications)
}

func TestGetStatsFallsBackWhenWorkspaceReadModelProjectTotalMismatchesFacts(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	userID := uuid.New()
	workspaceID := uuid.New()
	now := time.Now().UTC()

	require.NoError(t, db.Create(&models.User{ID: userID, Username: "stats-mismatch", Email: "stats-mismatch@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.Workspace{ID: workspaceID, OwnerUserID: userID, Name: "Stats mismatch", Status: models.WorkspaceStatusActive, CreatedAt: now, UpdatedAt: now}).Error)
	for range 2 {
		require.NoError(t, db.Create(&models.Project{
			ID:            uuid.New(),
			UserID:        userID,
			WorkspaceID:   &workspaceID,
			Title:         "Stats mismatch fact",
			SourceContent: "content",
			Status:        models.ProjectStatusReady,
			CreatedAt:     now,
			UpdatedAt:     now,
		}).Error)
	}
	require.NoError(t, db.Create(&models.WorkspaceDashboardStats{
		WorkspaceID:   workspaceID,
		TotalProjects: 1,
		TotalMembers:  1,
		RefreshedAt:   now,
	}).Error)

	stats, err := s.GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(0), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
}

func TestGetStatsCachesGlobalDashboardStats(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient, redisServer := newStatsRedisClientWithServer(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	seedStatsOverviewProject(t, db, "cached-a", models.PublicationStatusPublished)

	stats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
	cacheKey := requireSingleStatsCacheKey(t, redisClient)
	cacheTTL, err := redisClient.PTTL(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Positive(t, cacheTTL)
	require.LessOrEqual(t, cacheTTL, 15*time.Second)

	seedStatsOverviewProject(t, db, "cached-b", models.PublicationStatusFailed)

	cachedStats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), cachedStats.TotalUsers)
	assert.Equal(t, int64(1), cachedStats.TotalProjects)
	assert.Equal(t, int64(1), cachedStats.TotalPublishedPublications)
	assert.Equal(t, int64(0), cachedStats.TotalFailedPublications)

	redisServer.FastForward(16 * time.Second)

	refreshedStats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), refreshedStats.TotalUsers)
	assert.Equal(t, int64(2), refreshedStats.TotalProjects)
	assert.Equal(t, int64(1), refreshedStats.TotalPublishedPublications)
	assert.Equal(t, int64(1), refreshedStats.TotalFailedPublications)
}

func TestCreateProjectInvalidatesDashboardStatsCache(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedStatsOverviewProject(t, db, "stats-create", models.PublicationStatusPublished)

	stats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalProjects)
	staleKey := requireSingleStatsCacheKey(t, redisClient)

	_, err = s.WithContext(context.Background()).CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "stats-create-fresh",
		SourceContent: "content",
		Platforms:     []string{"zhihu"},
	})
	require.NoError(t, err)
	require.Contains(t, requireStatsCacheKeys(t, redisClient, 1), staleKey)

	refreshed, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), refreshed.TotalProjects)
	requireStatsCacheKeys(t, redisClient, 2)
}

func TestStatsCacheIgnoresStaleRefillAfterInvalidation(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)
	ctx := context.Background()

	user := seedStatsOverviewProject(t, db, "stats-cache-race", models.PublicationStatusPublished)

	first, err := s.WithContext(ctx).GetStats(nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.TotalProjects)
	staleKey := requireSingleStatsCacheKey(t, redisClient)
	stalePayload, err := redisClient.Get(ctx, staleKey).Bytes()
	require.NoError(t, err)

	_, err = s.WithContext(ctx).CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "stats-cache-fresh",
		SourceContent: "content",
		Platforms:     []string{"zhihu"},
	})
	require.NoError(t, err)
	requireStatsCacheKeys(t, redisClient, 1)

	require.NoError(t, redisClient.Set(ctx, staleKey, stalePayload, time.Minute).Err())

	refreshed, err := s.WithContext(ctx).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), refreshed.TotalProjects)
	assert.Equal(t, int64(1), refreshed.TotalUsers)
}

func TestUpdateProjectInvalidatesDashboardStatsCache(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)
	ctx := context.Background()

	user, project := seedStatsLifecycleProject(t, db, "stats-update")

	stats, err := s.WithContext(ctx).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)
	staleKey := requireSingleStatsCacheKey(t, redisClient)

	_, err = s.WithContext(ctx).UpdateProject(project.ID, user.ID, dto.UpdateProjectRequest{
		Title:         "stats-update-fresh",
		SourceContent: "<p>fresh</p>",
		Platforms:     []string{"zhihu", "douyin"},
	})
	require.NoError(t, err)
	require.Contains(t, requireStatsCacheKeys(t, redisClient, 1), staleKey)

	refreshed, err := s.WithContext(ctx).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), refreshed.TotalPublishedPublications)
	assert.Equal(t, int64(0), refreshed.TotalFailedPublications)
}

func TestSaveProjectPlatformsInvalidatesDashboardStatsCache(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)
	ctx := context.Background()

	user, project := seedStatsLifecycleProject(t, db, "stats-platforms")

	stats, err := s.WithContext(ctx).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)
	staleKey := requireSingleStatsCacheKey(t, redisClient)

	_, err = s.WithContext(ctx).SaveProjectPlatforms(project.ID, user.ID, dto.SaveProjectPlatformsRequest{
		Platforms: []string{"douyin"},
	})
	require.NoError(t, err)
	require.Contains(t, requireStatsCacheKeys(t, redisClient, 1), staleKey)

	refreshed, err := s.WithContext(ctx).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), refreshed.TotalPublishedPublications)
	assert.Equal(t, int64(0), refreshed.TotalFailedPublications)
}

func TestGetStatsBypassesCacheForStickyEventualCounts(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	s := services.NewDashboardServiceWithRouter(writer, router)
	s.UseRedis(redisClient)

	seedStatsOverviewProject(t, reader, "reader", models.PublicationStatusFailed)
	readerStats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), readerStats.TotalFailedPublications)

	seedStatsOverviewProject(t, writer, "writer", models.PublicationStatusPublished)
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))

	stickyStats, err := s.WithContext(stickyCtx).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stickyStats.TotalUsers)
	assert.Equal(t, int64(1), stickyStats.TotalProjects)
	assert.Equal(t, int64(1), stickyStats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stickyStats.TotalFailedPublications)
}

func TestGetStatsFallsBackToDatabaseWhenCachedPayloadIsInvalid(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	seedStatsOverviewProject(t, db, "fallback", models.PublicationStatusPublished)
	_, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	cacheKey := requireSingleStatsCacheKey(t, redisClient)
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, "not-json", time.Minute).Err())

	stats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)

	repairedPayload, err := redisClient.Get(context.Background(), cacheKey).Bytes()
	require.NoError(t, err)
	var repairedPayloadBody struct {
		Version int `json:"version"`
		Stats   struct {
			TotalUsers                 int64 `json:"total_users"`
			TotalProjects              int64 `json:"total_projects"`
			TotalPublishedPublications int64 `json:"total_published_publications"`
			TotalFailedPublications    int64 `json:"total_failed_publications"`
		} `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(repairedPayload, &repairedPayloadBody))
	assert.Equal(t, 1, repairedPayloadBody.Version)
	assert.Equal(t, int64(1), repairedPayloadBody.Stats.TotalUsers)
	assert.Equal(t, int64(1), repairedPayloadBody.Stats.TotalProjects)
	cacheTTL, err := redisClient.PTTL(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Positive(t, cacheTTL)
	require.LessOrEqual(t, cacheTTL, 15*time.Second)
}

func TestGetStatsRepairsSemanticallyInvalidCachedPayload(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	seedStatsOverviewProject(t, db, "semantic-invalid-a", models.PublicationStatusPublished)
	_, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	cacheKey := requireSingleStatsCacheKey(t, redisClient)
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, `{}`, time.Minute).Err())

	seedStatsOverviewProject(t, db, "semantic-invalid-b", models.PublicationStatusFailed)

	stats, err := s.WithContext(context.Background()).GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)

	repairedPayload, err := redisClient.Get(context.Background(), cacheKey).Bytes()
	require.NoError(t, err)
	var payload struct {
		Version int `json:"version"`
		Stats   struct {
			TotalProjects int64 `json:"total_projects"`
		} `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(repairedPayload, &payload))
	assert.Equal(t, 1, payload.Version)
	assert.Equal(t, int64(2), payload.Stats.TotalProjects)
}

func TestGetStatsDoesNotCacheScopedCounts(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedStatsOverviewProject(t, db, "scoped-a", models.PublicationStatusPublished)

	stats, err := s.WithContext(context.Background()).GetStats(&user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalProjects)

	project := models.Project{
		UserID:        user.ID,
		Title:         "scoped-b-project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	freshStats, err := s.WithContext(context.Background()).GetStats(&user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), freshStats.TotalUsers)
	assert.Equal(t, int64(2), freshStats.TotalProjects)
	assert.Equal(t, int64(1), freshStats.TotalPublishedPublications)
	assert.Equal(t, int64(1), freshStats.TotalFailedPublications)
}

func TestGetWorkspaceStatsFallbackIncludesLegacyPersonalProjects(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	now := time.Now().UTC()
	user := models.User{
		ID:           uuid.New(),
		Username:     "workspace-legacy-owner",
		Email:        "workspace-legacy-owner@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	workspaceID := models.PersonalWorkspaceID(user.ID)
	require.NoError(t, db.Create(&models.Workspace{
		ID:          workspaceID,
		OwnerUserID: user.ID,
		Name:        models.PersonalWorkspaceName,
		Status:      models.WorkspaceStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error)
	project := models.Project{
		ID:            uuid.New(),
		UserID:        user.ID,
		Title:         "Legacy personal project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:        uuid.New(),
		ProjectID: project.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:        uuid.New(),
		ProjectID: project.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	stats, err := s.GetWorkspaceStats(workspaceID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)
}

func TestGetStatsUsesReaderForEventualCounts(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	u1 := models.User{Username: "reader-user"}
	require.NoError(t, reader.Create(&u1).Error)
	project := models.Project{UserID: u1.ID, Title: "reader-project", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&project).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	var writerUsers int64
	require.NoError(t, writer.Model(&models.User{}).Count(&writerUsers).Error)
	require.Equal(t, int64(0), writerUsers)

	stats, err := s.GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
}

func TestGetStatsCollapsesConcurrentCacheMisses(t *testing.T) {
	db := testsupport.SetupTestDB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	redisClient := newStatsRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	seedStatsOverviewProject(t, db, "stats-singleflight", models.PublicationStatusPublished)

	queryCount := registerBlockingStatsQueryCounter(t, db)
	results := runConcurrentStatsRequests(t, s, queryCount)

	for err := range results.errs {
		require.NoError(t, err)
	}
	for stats := range results.stats {
		assert.Equal(t, int64(1), stats.TotalUsers)
		assert.Equal(t, int64(1), stats.TotalProjects)
		assert.Equal(t, int64(1), stats.TotalPublishedPublications)
		assert.Equal(t, int64(0), stats.TotalFailedPublications)
	}
	assert.Equal(t, int64(5), queryCount.count.Load())
	requireSingleStatsCacheKey(t, redisClient)
}

func TestGetStatsCollapsesConcurrentRedisReadErrors(t *testing.T) {
	db := testsupport.SetupTestDB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	redisClient, redisServer := newStatsRedisClientWithServer(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	seedStatsOverviewProject(t, db, "stats-redis-error", models.PublicationStatusFailed)
	redisServer.SetError("ERR forced")

	queryCount := registerBlockingStatsQueryCounter(t, db)
	results := runConcurrentStatsRequests(t, s, queryCount)

	for err := range results.errs {
		require.NoError(t, err)
	}
	for stats := range results.stats {
		assert.Equal(t, int64(1), stats.TotalUsers)
		assert.Equal(t, int64(1), stats.TotalProjects)
		assert.Equal(t, int64(0), stats.TotalPublishedPublications)
		assert.Equal(t, int64(1), stats.TotalFailedPublications)
	}
	assert.Equal(t, int64(5), queryCount.count.Load())
}

func newStatsRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	client, _ := newStatsRedisClientWithServer(t)
	return client
}

func newStatsRedisClientWithServer(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client, redisServer
}

func requireSingleStatsCacheKey(t *testing.T, client *redis.Client) string {
	t.Helper()

	return requireStatsCacheKeys(t, client, 1)[0]
}

func requireStatsCacheKeys(t *testing.T, client *redis.Client, count int) []string {
	t.Helper()

	cacheKeys, err := client.Keys(context.Background(), "mpp:dashboard:stats:*").Result()
	require.NoError(t, err)
	require.Len(t, cacheKeys, count)
	return cacheKeys
}

type statsQueryCounter struct {
	count             atomic.Int64
	released          atomic.Bool
	firstQuery        chan struct{}
	duplicateQuery    chan struct{}
	releaseFirstQuery chan struct{}
}

func registerBlockingStatsQueryCounter(t *testing.T, db *gorm.DB) *statsQueryCounter {
	t.Helper()

	counter := &statsQueryCounter{
		firstQuery:        make(chan struct{}),
		duplicateQuery:    make(chan struct{}),
		releaseFirstQuery: make(chan struct{}),
	}
	var closeFirstQuery sync.Once
	var closeDuplicateQuery sync.Once
	callbackName := "test:dashboard_stats_singleflight:" + t.Name()
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		count := counter.count.Add(1)
		if count == 1 {
			closeFirstQuery.Do(func() { close(counter.firstQuery) })
			<-counter.releaseFirstQuery
			return
		}
		if !counter.released.Load() {
			closeDuplicateQuery.Do(func() { close(counter.duplicateQuery) })
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})
	return counter
}

type concurrentStatsResults struct {
	errs  chan error
	stats chan *dto.DashboardStatsResponse
}

func runConcurrentStatsRequests(t *testing.T, s *services.DashboardService, queryCount *statsQueryCounter) concurrentStatsResults {
	t.Helper()

	const waitingCallers = 7
	errs := make(chan error, waitingCallers+1)
	statsCh := make(chan *dto.DashboardStatsResponse, waitingCallers+1)
	var wg sync.WaitGroup
	wg.Go(func() {
		stats, err := s.WithContext(context.Background()).GetStats(nil)
		if err != nil {
			errs <- err
			return
		}
		statsCh <- stats
	})

	select {
	case <-queryCount.firstQuery:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first stats query")
	}

	start := make(chan struct{})
	ready := make(chan struct{}, waitingCallers)
	for range waitingCallers {
		wg.Go(func() {
			<-start
			ready <- struct{}{}
			stats, err := s.WithContext(context.Background()).GetStats(nil)
			if err != nil {
				errs <- err
				return
			}
			statsCh <- stats
		})
	}

	close(start)
	for range waitingCallers {
		select {
		case <-ready:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for waiting stats caller")
		}
	}

	select {
	case <-queryCount.duplicateQuery:
		t.Fatal("stats cache refresh should share the active singleflight query")
	case <-time.After(50 * time.Millisecond):
	}
	queryCount.released.Store(true)
	close(queryCount.releaseFirstQuery)
	wg.Wait()
	close(errs)
	close(statsCh)
	return concurrentStatsResults{errs: errs, stats: statsCh}
}

func seedStatsOverviewProject(t *testing.T, db *gorm.DB, prefix string, publicationStatus string) models.User {
	t.Helper()

	user := models.User{
		Username:     prefix + "-user",
		Email:        prefix + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         prefix + "-project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Status:    publicationStatus,
	}).Error)
	return user
}

func seedStatsLifecycleProject(t *testing.T, db *gorm.DB, prefix string) (models.User, models.Project) {
	t.Helper()

	user := models.User{
		Username:     prefix + "-user",
		Email:        prefix + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         prefix + "-project",
		SourceContent: "<p>content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "zhihu",
		Enabled:   true,
		Status:    models.PublicationStatusFailed,
	}).Error)
	return user, project
}

func TestGetStatsUsesWriterForStickyEventualCounts(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))
	s := services.NewDashboardServiceWithRouter(writer, router).WithContext(stickyCtx)

	writerUser := models.User{Username: "writer-user", Email: "writer-user@example.com", PasswordHash: "hash"}
	require.NoError(t, writer.Create(&writerUser).Error)
	writerProject := models.Project{
		UserID:        writerUser.ID,
		Title:         "writer-project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, writer.Create(&writerProject).Error)
	require.NoError(t, writer.Create(&models.ProjectPlatformPublication{
		ProjectID: writerProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	readerUser := models.User{Username: "stale-reader-user", Email: "stale-reader-user@example.com", PasswordHash: "hash"}
	require.NoError(t, reader.Create(&readerUser).Error)
	staleReaderProject := models.Project{
		UserID:        readerUser.ID,
		Title:         "stale-reader-project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, reader.Create(&staleReaderProject).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: staleReaderProject.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	stats, err := s.GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
}

func TestGetStatsUsesWriterForScopedCounts(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	user := models.User{Username: "scoped-user"}
	require.NoError(t, writer.Create(&user).Error)
	currentProject := models.Project{UserID: user.ID, Title: "current-project", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, writer.Create(&currentProject).Error)
	require.NoError(t, writer.Create(&models.ProjectPlatformPublication{
		ProjectID: currentProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	staleProject := models.Project{UserID: user.ID, Title: "stale-reader-project", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&staleProject).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: staleProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: staleProject.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	stats, err := s.GetStats(&user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
}
