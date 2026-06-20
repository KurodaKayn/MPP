package contentsetup

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestContentSetupOptionsCacheCollapsesGenerationDegrade(t *testing.T) {
	t.Setenv("REDIS_DEGRADE_DASHBOARD_CONTENT_SETUP_CACHE_FAILURE_THRESHOLD", "1")
	t.Setenv("REDIS_DEGRADE_DASHBOARD_CONTENT_SETUP_CACHE_COOLDOWN", "5s")

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, redisClient.Close())
	})

	s := &Service{
		db:                testsupport.SetupTestDB(),
		cache:             redisClient,
		cacheTTL:          time.Minute,
		cacheGroup:        &singleflight.Group{},
		contentSetupGuard: redisdegrade.NewGuard(redisdegrade.GroupDashboardContentSetupCache),
	}
	userID := uuid.New()
	workspaceID := uuid.New()
	response := &dto.ContentTemplatesResponse{
		Items: []dto.ContentTemplate{{ID: uuid.New(), Name: "template"}},
	}
	valid := func(resp dto.ContentTemplatesResponse) bool {
		return resp.Items != nil
	}

	var computeCount atomic.Int64
	var block atomic.Bool
	started := make(chan struct{})
	release := make(chan struct{})
	var closeStarted sync.Once
	compute := func(*Service) (*dto.ContentTemplatesResponse, error) {
		count := computeCount.Add(1)
		if block.Load() && count == 1 {
			closeStarted.Do(func() { close(started) })
			<-release
		}
		return response, nil
	}

	redisServer.SetError("LOADING Redis is loading the dataset in memory")
	_, err := getCachedContentSetupOptions(s, contentSetupResourceTemplates, userID, workspaceID, compute, valid)
	require.NoError(t, err)
	redisServer.SetError("")

	computeCount.Store(0)
	block.Store(true)
	const callers = 8
	errs := make(chan error, callers)
	results := make(chan *dto.ContentTemplatesResponse, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			resp, err := getCachedContentSetupOptions(s, contentSetupResourceTemplates, userID, workspaceID, compute, valid)
			if err != nil {
				errs <- err
				return
			}
			results <- resp
		}()
	}

	close(start)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for degraded content setup compute")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		require.NoError(t, err)
	}
	for resp := range results {
		require.Equal(t, response, resp)
	}
	require.LessOrEqual(t, computeCount.Load(), int64(3))
}
