package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestApplicationRateLimiterBlocksAfterUserMinuteLimit(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.GeneralUserPerMinute = 2

	userID := uuid.New()
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodGet, "/api/user/dashboard/stats"))
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodGet, "/api/user/dashboard/stats"))

	status, headers, body := performRateLimitedRequestWithDetails(t, config, userID, "", http.MethodGet, "/api/user/dashboard/stats")

	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "0", headers.Get("X-RateLimit-Remaining"))
	require.NotEmpty(t, headers.Get("Retry-After"))
	require.Contains(t, body, `"code":"rate_limited"`)
}

func TestApplicationRateLimiterKeepsUserBucketsSeparate(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.GeneralUserPerMinute = 1

	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, uuid.New(), "", http.MethodGet, "/api/user/dashboard/stats"))
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, uuid.New(), "", http.MethodGet, "/api/user/dashboard/stats"))
}

func TestApplicationRateLimiterSharesTenantBuckets(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.GeneralTenantPerMinute = 1

	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, uuid.New(), "tenant-acme", http.MethodGet, "/api/user/dashboard/stats"))
	require.Equal(t, http.StatusTooManyRequests, performRateLimitedRequest(t, config, uuid.New(), "tenant-acme", http.MethodGet, "/api/user/dashboard/stats"))
}

func TestApplicationRateLimiterSharesDefaultTenantBucketWhenClaimMissing(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.GeneralTenantPerMinute = 1

	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, uuid.New(), "", http.MethodGet, "/api/user/dashboard/stats"))
	require.Equal(t, http.StatusTooManyRequests, performRateLimitedRequest(t, config, uuid.New(), "", http.MethodGet, "/api/user/dashboard/stats"))
}

func TestApplicationRateLimiterUsesInterfaceBuckets(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.InterfaceUserPerMinute = 1

	userID := uuid.New()
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodGet, "/api/user/dashboard/stats"))
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodGet, "/api/user/dashboard/projects"))
	require.Equal(t, http.StatusTooManyRequests, performRateLimitedRequest(t, config, userID, "", http.MethodGet, "/api/user/dashboard/stats"))
}

func TestApplicationRateLimiterAppliesAIRouteQuota(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.AIUserPerMinute = 1

	userID := uuid.New()
	route := "/api/user/dashboard/ai/content/edit"
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodPost, route))

	status, headers, body := performRateLimitedRequestWithDetails(t, config, userID, "", http.MethodPost, route)

	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "ai", headers.Get("X-RateLimit-Bucket"))
	require.Contains(t, body, `"limit":1`)
}

func TestApplicationRateLimiterAppliesPublishRouteQuota(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.PublishUserPerMinute = 1

	userID := uuid.New()
	route := "/api/user/dashboard/projects/:id/publish"
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodPost, route))

	status, headers, body := performRateLimitedRequestWithDetails(t, config, userID, "", http.MethodPost, route)

	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "publish", headers.Get("X-RateLimit-Bucket"))
	require.Contains(t, body, `"limit":1`)
}

func TestApplicationRateLimiterAppliesBrowserSessionRouteQuota(t *testing.T) {
	client := setupRateLimitRedis(t)
	config := rateLimitTestConfig(client)
	config.BrowserSessionUserPerMinute = 1

	userID := uuid.New()
	route := "/api/user/dashboard/settings/platforms/:platform/browser-session"
	require.Equal(t, http.StatusOK, performRateLimitedRequest(t, config, userID, "", http.MethodPost, route))

	status, headers, body := performRateLimitedRequestWithDetails(t, config, userID, "", http.MethodPost, route)

	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "browser_session", headers.Get("X-RateLimit-Bucket"))
	require.Contains(t, body, `"limit":1`)
}

func TestDefaultRateLimitConfigLoadsEmbeddedPolicy(t *testing.T) {
	config := DefaultRateLimitConfig(nil)

	require.EqualValues(t, 600, config.GeneralUserPerMinute)
	require.EqualValues(t, 20, config.AIUserPerMinute)
	require.EqualValues(t, 5, config.PublishUserPerMinute)
	require.EqualValues(t, 3, config.BrowserSessionUserPerMinute)
}

func TestRateLimitConfigFromEnvHonorsDeploymentSwitches(t *testing.T) {
	t.Setenv("APP_RATE_LIMIT_ENABLED", "false")
	t.Setenv("APP_RATE_LIMIT_KEY_PREFIX", "custom:limits")

	config, err := RateLimitConfigFromEnv(&redis.Client{})

	require.NoError(t, err)
	require.False(t, config.Enabled)
	require.Equal(t, "custom:limits", config.KeyPrefix)
	require.Equal(t, DefaultRateLimitConfig(nil).AIUserPerMinute, config.AIUserPerMinute)
}

func setupRateLimitRedis(t *testing.T) *redis.Client {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}

func rateLimitTestConfig(client *redis.Client) RateLimitConfig {
	config := DefaultRateLimitConfig(client)
	config.GeneralUserPerMinute = 100
	config.GeneralTenantPerMinute = 100
	config.InterfaceUserPerMinute = 100
	config.InterfaceTenantPerMinute = 100
	config.AIUserPerMinute = 100
	config.AITenantPerMinute = 100
	config.AIUserPerDay = 100
	config.AITenantPerDay = 100
	config.PublishUserPerMinute = 100
	config.PublishTenantPerMinute = 100
	config.PublishUserPerDay = 100
	config.PublishTenantPerDay = 100
	config.BrowserSessionUserPerMinute = 100
	config.BrowserSessionTenantPerMinute = 100
	config.BrowserSessionUserPerDay = 100
	config.BrowserSessionTenantPerDay = 100
	return config
}

func performRateLimitedRequest(t *testing.T, config RateLimitConfig, userID uuid.UUID, tenantID, method, route string) int {
	t.Helper()

	status, _, _ := performRateLimitedRequestWithDetails(t, config, userID, tenantID, method, route)
	return status
}

func performRateLimitedRequestWithDetails(t *testing.T, config RateLimitConfig, userID uuid.UUID, tenantID, method, route string) (int, http.Header, string) {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(method, route, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(route)
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTCustomClaims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     "user",
	}))

	handler := ApplicationRateLimiter(config)(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	require.NoError(t, handler(c))
	return rec.Code, rec.Header(), strings.TrimSpace(rec.Body.String())
}
