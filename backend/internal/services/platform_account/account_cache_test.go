package platformaccount_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestDashboardAccountCacheCachesPersonalAccount(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient, redisServer := newAccountCacheRedisClientWithServer(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-hit")
	account := seedAccountCacheAccount(t, db, user, "douyin", "creator")

	first, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "creator", first.Username)
	cacheKey := requireSingleAccountCacheKey(t, redisClient)
	cacheTTL, err := redisClient.PTTL(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.Positive(t, cacheTTL)
	require.LessOrEqual(t, cacheTTL, 15*time.Second)

	require.NoError(t, db.Model(&account).Updates(map[string]any{
		"username":   "fresh",
		"avatar_url": "https://example.com/fresh.png",
	}).Error)

	cached, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "creator", cached.Username)

	redisServer.FastForward(16 * time.Second)

	refreshed, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "fresh", refreshed.Username)
	require.Equal(t, "https://example.com/fresh.png", refreshed.AvatarURL)
}

func TestDashboardAccountCacheBypassesStickyWriter(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newAccountCacheRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-sticky")
	account := seedAccountCacheAccount(t, db, user, "douyin", "cached")

	first, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "cached", first.Username)
	requireAccountCacheKeys(t, redisClient, 1)

	require.NoError(t, db.Model(&account).Update("username", "writer").Error)
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))

	sticky, err := s.WithContext(stickyCtx).GetDouyinAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "writer", sticky.Username)
}

func TestDashboardAccountCacheInvalidatesAfterWechatSaveAndTest(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newAccountCacheRedisClient(t)
	testedAt := time.Now()
	tester := &testsupport.FakeWechatTester{
		Result: dto.WechatConnectionTestResponse{
			Connected: true,
			Status:    models.PlatformAccountStatusConnected,
			Message:   "ok",
			TestedAt:  testedAt,
		},
	}
	s := services.NewDashboardServiceWithWechatTester(db, tester)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-wechat")
	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret",
	})
	require.NoError(t, err)

	cached, err := s.WithContext(context.Background()).GetWechatAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, models.PlatformAccountStatusUntested, cached.Status)
	requireAccountCacheKeys(t, redisClient, 1)

	_, err = s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{AppID: "wx-app"})
	require.NoError(t, err)
	requireAccountCacheKeys(t, redisClient, 0)

	tested, err := s.WithContext(context.Background()).GetWechatAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, models.PlatformAccountStatusConnected, tested.Status)
	requireAccountCacheKeys(t, redisClient, 1)

	_, err = s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret-new",
	})
	require.NoError(t, err)
	requireAccountCacheKeys(t, redisClient, 0)

	saved, err := s.WithContext(context.Background()).GetWechatAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, models.PlatformAccountStatusUntested, saved.Status)
}

func TestDashboardAccountCacheRepairsSemanticallyInvalidPayload(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newAccountCacheRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-repair")
	account := seedAccountCacheAccount(t, db, user, "zhihu", "cached")

	first, err := s.WithContext(context.Background()).GetZhihuAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "cached", first.Username)
	cacheKey := requireSingleAccountCacheKey(t, redisClient)
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, `{}`, time.Minute).Err())

	require.NoError(t, db.Model(&account).Update("username", "fresh").Error)

	repaired, err := s.WithContext(context.Background()).GetZhihuAccount(user.ID)
	require.NoError(t, err)
	require.Equal(t, "fresh", repaired.Username)

	payloadBytes, err := redisClient.Get(context.Background(), cacheKey).Bytes()
	require.NoError(t, err)
	var payload struct {
		Version  int `json:"version"`
		Response struct {
			Platform string `json:"platform"`
			Username string `json:"username"`
			Status   string `json:"status"`
		} `json:"response"`
	}
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	require.Equal(t, 1, payload.Version)
	require.Equal(t, "zhihu", payload.Response.Platform)
	require.Equal(t, "fresh", payload.Response.Username)
	require.Equal(t, models.PlatformAccountStatusConnected, payload.Response.Status)
}

func TestDashboardAccountCacheCollapsesConcurrentMisses(t *testing.T) {
	db := testsupport.SetupTestDB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	redisClient := newAccountCacheRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-singleflight")
	seedAccountCacheAccount(t, db, user, "douyin", "singleflight")

	var queryCount atomic.Int64
	firstQuery := make(chan struct{})
	releaseFirstQuery := make(chan struct{})
	var closeFirstQuery sync.Once
	const callbackName = "test:dashboard_account_singleflight"
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
	usernames := make(chan string, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			resp, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
			if err != nil {
				errs <- err
				return
			}
			usernames <- resp.Username
		}()
	}

	close(start)
	select {
	case <-firstQuery:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first account query")
	}
	close(releaseFirstQuery)
	wg.Wait()
	close(errs)
	close(usernames)

	for err := range errs {
		require.NoError(t, err)
	}
	for username := range usernames {
		require.Equal(t, "singleflight", username)
	}
	require.Equal(t, int64(1), queryCount.Load())
	requireSingleAccountCacheKey(t, redisClient)
}

func TestDashboardAccountCacheCollapsesConcurrentRedisReadErrors(t *testing.T) {
	db := testsupport.SetupTestDB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	redisClient, redisServer := newAccountCacheRedisClientWithServer(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-redis-error")
	seedAccountCacheAccount(t, db, user, "douyin", "redis-error")
	redisServer.SetError("ERR forced")

	var queryCount atomic.Int64
	firstQuery := make(chan struct{})
	releaseFirstQuery := make(chan struct{})
	var closeFirstQuery sync.Once
	const callbackName = "test:dashboard_account_redis_error_singleflight"
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
	usernames := make(chan string, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			resp, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
			if err != nil {
				errs <- err
				return
			}
			usernames <- resp.Username
		}()
	}

	close(start)
	select {
	case <-firstQuery:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first account query")
	}
	close(releaseFirstQuery)
	wg.Wait()
	close(errs)
	close(usernames)

	for err := range errs {
		require.NoError(t, err)
	}
	for username := range usernames {
		require.Equal(t, "redis-error", username)
	}
	require.Equal(t, int64(1), queryCount.Load())
}

func TestDashboardAccountCacheRefreshSurvivesFirstCallerCancel(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newAccountCacheRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := seedAccountCacheUser(t, db, "cache-cancel")
	seedAccountCacheAccount(t, db, user, "douyin", "refresh")

	var queryCount atomic.Int64
	firstQuery := make(chan struct{})
	secondRefreshStarted := make(chan struct{})
	releaseQueries := make(chan struct{})
	var closeFirstQuery sync.Once
	var closeSecondRefresh sync.Once
	var blocking atomic.Bool
	blocking.Store(true)
	const callbackName = "test:dashboard_account_cancel"
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
		_, err := s.WithContext(firstCtx).GetDouyinAccount(user.ID)
		firstErr <- err
	}()

	select {
	case <-firstQuery:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first account query")
	}

	secondResult := make(chan *dto.DouyinAccountResponse, 1)
	secondErr := make(chan error, 1)
	go func() {
		resp, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
		if err != nil {
			secondErr <- err
			return
		}
		secondResult <- resp
	}()

	time.Sleep(20 * time.Millisecond)
	cancelFirst()
	require.ErrorIs(t, <-firstErr, context.Canceled)

	thirdResult := make(chan *dto.DouyinAccountResponse, 1)
	thirdErr := make(chan error, 1)
	thirdStarted := make(chan struct{})
	go func() {
		close(thirdStarted)
		resp, err := s.WithContext(context.Background()).GetDouyinAccount(user.ID)
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
		require.Equal(t, "refresh", resp.Username)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second account result")
	}
	select {
	case err := <-thirdErr:
		require.NoError(t, err)
	case resp := <-thirdResult:
		require.Equal(t, "refresh", resp.Username)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for third account result")
	}
	requireSingleAccountCacheKey(t, redisClient)
	require.Equal(t, int64(1), queryCount.Load())
}

func newAccountCacheRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	client, _ := newAccountCacheRedisClientWithServer(t)
	return client
}

func newAccountCacheRedisClientWithServer(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client, redisServer
}

func requireSingleAccountCacheKey(t *testing.T, client *redis.Client) string {
	t.Helper()

	cacheKeys, err := client.Keys(context.Background(), "mpp:dashboard:accounts:*").Result()
	require.NoError(t, err)
	require.Len(t, cacheKeys, 1)
	return cacheKeys[0]
}

func requireAccountCacheKeys(t *testing.T, client *redis.Client, count int) {
	t.Helper()

	cacheKeys, err := client.Keys(context.Background(), "mpp:dashboard:accounts:*").Result()
	require.NoError(t, err)
	require.Len(t, cacheKeys, count)
}

func seedAccountCacheUser(t *testing.T, db *gorm.DB, prefix string) models.User {
	t.Helper()

	user := models.User{
		Username:     prefix + "-user",
		Email:        prefix + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func seedAccountCacheAccount(t *testing.T, db *gorm.DB, user models.User, platform string, username string) models.PlatformAccount {
	t.Helper()

	account := models.PlatformAccount{
		UserID:       user.ID,
		Platform:     platform,
		Username:     username,
		DisplayName:  username,
		AvatarURL:    "https://example.com/" + username + ".png",
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
		Credentials:  datatypes.JSON([]byte(`{}`)),
		Metadata:     datatypes.JSON([]byte(`{}`)),
	}
	require.NoError(t, db.Create(&account).Error)
	return account
}
