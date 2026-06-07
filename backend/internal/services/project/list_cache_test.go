package project_test

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestListProjectsCachesAdminDashboardList(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient, redisServer := newProjectListRedisClientWithServer(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-admin")
	firstProject := seedProjectListCacheProject(t, db, user, "cached-a", models.ProjectStatusReady, "wechat")

	first, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Total)
	firstItems := first.Items.([]dto.ProjectListItem)
	require.Len(t, firstItems, 1)
	require.Equal(t, firstProject.ID, firstItems[0].ID)
	cacheKey := requireSingleProjectListCacheKey(t, redisClient)
	cacheTTL, err := redisClient.PTTL(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Positive(t, cacheTTL)
	require.LessOrEqual(t, cacheTTL, 15*time.Second)

	seedProjectListCacheProject(t, db, user, "cached-b", models.ProjectStatusReady, "zhihu")

	cached, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), cached.Total)
	cachedItems := cached.Items.([]dto.ProjectListItem)
	require.Len(t, cachedItems, 1)
	require.Equal(t, firstProject.ID, cachedItems[0].ID)

	redisServer.FastForward(16 * time.Second)

	refreshed, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), refreshed.Total)
}

func TestListProjectsCacheSeparatesFilters(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newProjectListRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-filter")
	readyProject := seedProjectListCacheProject(t, db, user, "ready", models.ProjectStatusReady, "wechat")
	draftProject := seedProjectListCacheProject(t, db, user, "draft", models.ProjectStatusDraft, "zhihu")

	ready, err := s.WithContext(context.Background()).ListProjects(1, 10, models.ProjectStatusReady, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), ready.Total)
	readyItems := ready.Items.([]dto.ProjectListItem)
	require.Len(t, readyItems, 1)
	require.Equal(t, readyProject.ID, readyItems[0].ID)

	draft, err := s.WithContext(context.Background()).ListProjects(1, 10, models.ProjectStatusDraft, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), draft.Total)
	draftItems := draft.Items.([]dto.ProjectListItem)
	require.Len(t, draftItems, 1)
	require.Equal(t, draftProject.ID, draftItems[0].ID)

	requireProjectListCacheKeys(t, redisClient, 2)
}

func TestListProjectsCacheBypassesScopedLists(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newProjectListRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-scoped")
	seedProjectListCacheProject(t, db, user, "admin-cached", models.ProjectStatusReady, "wechat")

	admin, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), admin.Total)
	requireProjectListCacheKeys(t, redisClient, 1)

	seedProjectListCacheProject(t, db, user, "scoped-fresh", models.ProjectStatusReady, "zhihu")

	scoped, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", &user.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), scoped.Total)
	requireProjectListCacheKeys(t, redisClient, 1)
}

func TestListProjectsCacheBypassesStickyWriter(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	redisClient := newProjectListRedisClient(t)
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	s := services.NewDashboardServiceWithRouter(writer, router)
	s.UseRedis(redisClient)

	readerUser := createProjectListCacheUser(t, reader, "cache-reader")
	readerProject := seedProjectListCacheProject(t, reader, readerUser, "reader-cached", models.ProjectStatusReady, "wechat")

	readerList, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), readerList.Total)
	readerItems := readerList.Items.([]dto.ProjectListItem)
	require.Len(t, readerItems, 1)
	require.Equal(t, readerProject.ID, readerItems[0].ID)
	requireProjectListCacheKeys(t, redisClient, 1)

	writerUser := createProjectListCacheUser(t, writer, "cache-writer")
	writerProject := seedProjectListCacheProject(t, writer, writerUser, "writer-fresh", models.ProjectStatusReady, "zhihu")
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))

	stickyList, err := s.WithContext(stickyCtx).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), stickyList.Total)
	stickyItems := stickyList.Items.([]dto.ProjectListItem)
	require.Len(t, stickyItems, 1)
	require.Equal(t, writerProject.ID, stickyItems[0].ID)
}

func TestListProjectsCacheRepairsInvalidPayload(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newProjectListRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-repair")
	seedProjectListCacheProject(t, db, user, "cached", models.ProjectStatusReady, "wechat")

	_, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	cacheKey := requireSingleProjectListCacheKey(t, redisClient)
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, "not-json", time.Minute).Err())

	seedProjectListCacheProject(t, db, user, "fresh", models.ProjectStatusReady, "zhihu")

	repaired, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), repaired.Total)

	payloadBytes, err := redisClient.Get(context.Background(), cacheKey).Bytes()
	require.NoError(t, err)
	var payload struct {
		Items []dto.ProjectListItem `json:"items"`
		Total int64                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	require.Equal(t, int64(2), payload.Total)
	require.Len(t, payload.Items, 2)
	cacheTTL, err := redisClient.PTTL(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Positive(t, cacheTTL)
	require.LessOrEqual(t, cacheTTL, 15*time.Second)
}

func TestListProjectsCacheRepairsSemanticallyInvalidPayload(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newProjectListRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-semantic")
	seedProjectListCacheProject(t, db, user, "cached", models.ProjectStatusReady, "wechat")

	_, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	cacheKey := requireSingleProjectListCacheKey(t, redisClient)
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, `{}`, time.Minute).Err())

	seedProjectListCacheProject(t, db, user, "fresh", models.ProjectStatusReady, "zhihu")

	repaired, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), repaired.Total)

	payloadBytes, err := redisClient.Get(context.Background(), cacheKey).Bytes()
	require.NoError(t, err)
	var payload struct {
		Items []dto.ProjectListItem `json:"items"`
		Page  int                   `json:"page"`
		Limit int                   `json:"limit"`
		Total int64                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	require.Equal(t, 1, payload.Page)
	require.Equal(t, 10, payload.Limit)
	require.Equal(t, int64(2), payload.Total)
	require.Len(t, payload.Items, 2)
}

func TestListProjectsCacheCollapsesConcurrentMisses(t *testing.T) {
	db := testsupport.SetupTestDB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	redisClient := newProjectListRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-singleflight")
	seedProjectListCacheProject(t, db, user, "singleflight", models.ProjectStatusReady, "wechat")

	var queryCount atomic.Int64
	firstQuery := make(chan struct{})
	releaseFirstQuery := make(chan struct{})
	var closeFirstQuery sync.Once
	const callbackName = "test:dashboard_project_list_singleflight"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		count := queryCount.Add(1)
		if count == 1 {
			closeFirstQuery.Do(func() { close(firstQuery) })
			<-releaseFirstQuery
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	const callers = 8
	start := make(chan struct{})
	errs := make(chan error, callers)
	totals := make(chan int64, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			resp, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
			if err != nil {
				errs <- err
				return
			}
			totals <- resp.Total
		}()
	}

	close(start)
	select {
	case <-firstQuery:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first project list query")
	}
	close(releaseFirstQuery)
	wg.Wait()
	close(errs)
	close(totals)

	for err := range errs {
		require.NoError(t, err)
	}
	for total := range totals {
		require.Equal(t, int64(1), total)
	}
	require.LessOrEqual(t, queryCount.Load(), int64(3))
}

func TestListProjectsCacheRefreshSurvivesFirstCallerCancel(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newProjectListRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createProjectListCacheUser(t, db, "cache-cancel")
	seedProjectListCacheProject(t, db, user, "refresh", models.ProjectStatusReady, "wechat")

	var queryCount atomic.Int64
	firstQuery := make(chan struct{})
	secondRefreshStarted := make(chan struct{})
	releaseQueries := make(chan struct{})
	var closeFirstQuery sync.Once
	var closeSecondRefresh sync.Once
	var blocking atomic.Bool
	blocking.Store(true)
	const callbackName = "test:dashboard_project_list_cancel"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		count := queryCount.Add(1)
		if count > 1 && blocking.Load() {
			closeSecondRefresh.Do(func() { close(secondRefreshStarted) })
		}
		if blocking.Load() {
			closeFirstQuery.Do(func() { close(firstQuery) })
			<-releaseQueries
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := s.WithContext(firstCtx).ListProjects(1, 10, "", "", "", nil)
		firstErr <- err
	}()

	select {
	case <-firstQuery:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first project list query")
	}

	secondResult := make(chan *dto.PaginationResponse, 1)
	secondErr := make(chan error, 1)
	go func() {
		resp, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
		if err != nil {
			secondErr <- err
			return
		}
		secondResult <- resp
	}()

	time.Sleep(20 * time.Millisecond)
	cancelFirst()
	require.ErrorIs(t, <-firstErr, context.Canceled)

	thirdResult := make(chan *dto.PaginationResponse, 1)
	thirdErr := make(chan error, 1)
	thirdStarted := make(chan struct{})
	go func() {
		close(thirdStarted)
		resp, err := s.WithContext(context.Background()).ListProjects(1, 10, "", "", "", nil)
		if err != nil {
			thirdErr <- err
			return
		}
		thirdResult <- resp
	}()
	<-thirdStarted
	select {
	case <-secondRefreshStarted:
		t.Fatal("canceled caller should not forget active singleflight refresh")
	case <-time.After(50 * time.Millisecond):
	}

	blocking.Store(false)
	close(releaseQueries)

	select {
	case err := <-secondErr:
		require.NoError(t, err)
	case resp := <-secondResult:
		require.Equal(t, int64(1), resp.Total)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second project list result")
	}
	select {
	case err := <-thirdErr:
		require.NoError(t, err)
	case resp := <-thirdResult:
		require.Equal(t, int64(1), resp.Total)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for third project list result")
	}
	requireSingleProjectListCacheKey(t, redisClient)
	require.LessOrEqual(t, queryCount.Load(), int64(3))
}

func newProjectListRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	client, _ := newProjectListRedisClientWithServer(t)
	return client
}

func newProjectListRedisClientWithServer(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client, redisServer
}

func createProjectListCacheUser(t *testing.T, db *gorm.DB, prefix string) models.User {
	t.Helper()

	user := models.User{
		Username:     prefix + "-user",
		Email:        prefix + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func seedProjectListCacheProject(t *testing.T, db *gorm.DB, user models.User, title string, status string, platform string) models.Project {
	t.Helper()

	project := models.Project{
		UserID:        user.ID,
		Title:         title,
		SourceContent: "content",
		Status:        status,
		CreatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&project).Error)
	if platform != "" {
		require.NoError(t, db.Create(&models.ProjectPlatformPublication{
			ProjectID: project.ID,
			Platform:  platform,
			Status:    models.PublicationStatusPublished,
		}).Error)
	}
	return project
}

func requireSingleProjectListCacheKey(t *testing.T, client *redis.Client) string {
	t.Helper()

	return requireProjectListCacheKeys(t, client, 1)[0]
}

func requireProjectListCacheKeys(t *testing.T, client *redis.Client, count int) []string {
	t.Helper()

	cacheKeys, err := client.Keys(context.Background(), "mpp:dashboard:projects:list:*").Result()
	require.NoError(t, err)
	sort.Strings(cacheKeys)
	require.Len(t, cacheKeys, count)
	return cacheKeys
}
