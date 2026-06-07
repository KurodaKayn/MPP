package stats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
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
	var repairedStats map[string]any
	require.NoError(t, json.Unmarshal(repairedPayload, &repairedStats))
	cacheTTL, err := redisClient.PTTL(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Positive(t, cacheTTL)
	require.LessOrEqual(t, cacheTTL, 15*time.Second)
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

	cacheKeys, err := client.Keys(context.Background(), "mpp:dashboard:stats:*").Result()
	require.NoError(t, err)
	require.Len(t, cacheKeys, 1)
	return cacheKeys[0]
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
