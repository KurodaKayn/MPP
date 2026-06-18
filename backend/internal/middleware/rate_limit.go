package middleware

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"

	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
)

const (
	rateLimitEnabledEnv  = "APP_RATE_LIMIT_ENABLED"
	rateLimitPrefixEnv   = "APP_RATE_LIMIT_KEY_PREFIX"
	rateLimitFailOpenEnv = "APP_RATE_LIMIT_FAIL_OPEN"

	defaultRateLimitKeyPrefix = "mpp:ratelimit"
	defaultRateLimitTenantID  = "default"
)

//go:embed rate_limits.yml
var defaultRateLimitPolicyYAML []byte

type RateLimitConfig struct {
	Enabled     bool
	RedisClient *redis.Client
	KeyPrefix   string
	FailOpen    bool
	guard       *redisdegrade.Guard

	GeneralUserPerMinute     int64
	GeneralTenantPerMinute   int64
	InterfaceUserPerMinute   int64
	InterfaceTenantPerMinute int64

	AIUserPerMinute   int64
	AITenantPerMinute int64
	AIUserPerDay      int64
	AITenantPerDay    int64

	PublishUserPerMinute   int64
	PublishTenantPerMinute int64
	PublishUserPerDay      int64
	PublishTenantPerDay    int64

	BrowserSessionUserPerMinute   int64
	BrowserSessionTenantPerMinute int64
	BrowserSessionUserPerDay      int64
	BrowserSessionTenantPerDay    int64
}

type rateLimitBucket struct {
	Name       string
	Scope      string
	Identifier string
	Category   string
	Limit      int64
	Window     time.Duration
}

type rateLimitPolicy struct {
	General        rateLimitPolicyQuota `yaml:"general"`
	Interface      rateLimitPolicyQuota `yaml:"interface"`
	AI             rateLimitPolicyQuota `yaml:"ai"`
	Publish        rateLimitPolicyQuota `yaml:"publish"`
	BrowserSession rateLimitPolicyQuota `yaml:"browser_session"`
}

type rateLimitPolicyQuota struct {
	UserPerMinute   int64 `yaml:"user_per_minute"`
	TenantPerMinute int64 `yaml:"tenant_per_minute"`
	UserPerDay      int64 `yaml:"user_per_day"`
	TenantPerDay    int64 `yaml:"tenant_per_day"`
}

type rateLimitResult struct {
	Bucket     rateLimitBucket
	Current    int64
	Remaining  int64
	RetryAfter time.Duration
	Exceeded   bool
}

type rateLimitErrorResponse struct {
	Error rateLimitErrorBody `json:"error"`
}

type rateLimitErrorBody struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Limit             int64  `json:"limit"`
	Remaining         int64  `json:"remaining"`
	ResetAfterSeconds int64  `json:"reset_after_seconds"`
}

const redisRateLimitScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return { current, ttl }
`

func DefaultRateLimitConfig(client *redis.Client) RateLimitConfig {
	policy, err := rateLimitPolicyFromYAML(defaultRateLimitPolicyYAML)
	if err != nil {
		panic(err)
	}
	return rateLimitConfigFromPolicy(client, policy)
}

func rateLimitConfigFromPolicy(client *redis.Client, policy rateLimitPolicy) RateLimitConfig {
	return RateLimitConfig{
		Enabled:     client != nil,
		RedisClient: client,
		KeyPrefix:   defaultRateLimitKeyPrefix,
		FailOpen:    true,
		guard:       redisdegrade.NewGuard(redisdegrade.GroupRateLimit),

		GeneralUserPerMinute:     policy.General.UserPerMinute,
		GeneralTenantPerMinute:   policy.General.TenantPerMinute,
		InterfaceUserPerMinute:   policy.Interface.UserPerMinute,
		InterfaceTenantPerMinute: policy.Interface.TenantPerMinute,

		AIUserPerMinute:   policy.AI.UserPerMinute,
		AITenantPerMinute: policy.AI.TenantPerMinute,
		AIUserPerDay:      policy.AI.UserPerDay,
		AITenantPerDay:    policy.AI.TenantPerDay,

		PublishUserPerMinute:   policy.Publish.UserPerMinute,
		PublishTenantPerMinute: policy.Publish.TenantPerMinute,
		PublishUserPerDay:      policy.Publish.UserPerDay,
		PublishTenantPerDay:    policy.Publish.TenantPerDay,

		BrowserSessionUserPerMinute:   policy.BrowserSession.UserPerMinute,
		BrowserSessionTenantPerMinute: policy.BrowserSession.TenantPerMinute,
		BrowserSessionUserPerDay:      policy.BrowserSession.UserPerDay,
		BrowserSessionTenantPerDay:    policy.BrowserSession.TenantPerDay,
	}
}

func RateLimitConfigFromEnv(client *redis.Client) (RateLimitConfig, error) {
	policy, err := rateLimitPolicyFromYAML(defaultRateLimitPolicyYAML)
	if err != nil {
		return RateLimitConfig{}, err
	}

	config := rateLimitConfigFromPolicy(client, policy)
	config.Enabled = client != nil && envFlagEnabledDefault(rateLimitEnabledEnv, true)
	if prefix := strings.TrimSpace(os.Getenv(rateLimitPrefixEnv)); prefix != "" {
		config.KeyPrefix = prefix
	}
	config.FailOpen = envFlagEnabledDefault(rateLimitFailOpenEnv, true)

	return config, nil
}

func rateLimitPolicyFromYAML(raw []byte) (rateLimitPolicy, error) {
	var policy rateLimitPolicy
	if err := yaml.Unmarshal(raw, &policy); err != nil {
		return rateLimitPolicy{}, fmt.Errorf("parse rate limit policy: %w", err)
	}
	if err := validateRateLimitPolicy(policy); err != nil {
		return rateLimitPolicy{}, err
	}
	return policy, nil
}

func validateRateLimitPolicy(policy rateLimitPolicy) error {
	quotas := map[string]rateLimitPolicyQuota{
		"general":         policy.General,
		"interface":       policy.Interface,
		"ai":              policy.AI,
		"publish":         policy.Publish,
		"browser_session": policy.BrowserSession,
	}
	for name, quota := range quotas {
		if quota.UserPerMinute < 0 || quota.TenantPerMinute < 0 || quota.UserPerDay < 0 || quota.TenantPerDay < 0 {
			return fmt.Errorf("invalid rate limit policy: %s quota values must be non-negative", name)
		}
	}
	return nil
}

func ApplicationRateLimiter(config RateLimitConfig) echo.MiddlewareFunc {
	if !config.Enabled || config.RedisClient == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}
	if strings.TrimSpace(config.KeyPrefix) == "" {
		config.KeyPrefix = defaultRateLimitKeyPrefix
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			buckets, err := rateLimitBucketsFor(c, config)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, rateLimitErrorResponse{
					Error: rateLimitErrorBody{
						Code:    "unauthorized",
						Message: err.Error(),
					},
				})
			}
			if len(buckets) == 0 {
				return next(c)
			}

			result, err := checkRateLimitBuckets(c.Request().Context(), config.RedisClient, config.KeyPrefix, config.guard, buckets)
			if result.Exceeded {
				writeRateLimitHeaders(c, result)
				return rateLimitExceededResponse(c, result)
			}
			if err != nil {
				if config.FailOpen && redisdegrade.ShouldDegrade(err) {
					c.Response().Header().Set("X-RateLimit-Degraded", "true")
					return next(c)
				}
				return c.JSON(http.StatusServiceUnavailable, rateLimitErrorResponse{
					Error: rateLimitErrorBody{
						Code:    "rate_limit_unavailable",
						Message: "rate limiter is unavailable",
					},
				})
			}

			writeRateLimitHeaders(c, result)
			return next(c)
		}
	}
}

func rateLimitExceededResponse(c echo.Context, result rateLimitResult) error {
	return c.JSON(http.StatusTooManyRequests, rateLimitErrorResponse{
		Error: rateLimitErrorBody{
			Code:              "rate_limited",
			Message:           "rate limit exceeded",
			Limit:             result.Bucket.Limit,
			Remaining:         0,
			ResetAfterSeconds: resetAfterSeconds(result.RetryAfter),
		},
	})
}

func rateLimitBucketsFor(c echo.Context, config RateLimitConfig) ([]rateLimitBucket, error) {
	claims, err := jwtClaimsFromContext(c)
	if err != nil {
		return nil, err
	}

	userID := claims.UserID.String()
	tenantID := strings.TrimSpace(claims.TenantID)
	if tenantID == "" {
		tenantID = defaultRateLimitTenantID
	}

	method := strings.ToUpper(c.Request().Method)
	route := c.Path()
	if route == "" {
		route = c.Request().URL.Path
	}
	interfaceID := method + ":" + route
	category := rateLimitCategory(method, route)

	buckets := []rateLimitBucket{
		newRateLimitBucket("general:minute:user", "user", userID, "general", config.GeneralUserPerMinute, time.Minute),
		newRateLimitBucket("general:minute:tenant", "tenant", tenantID, "general", config.GeneralTenantPerMinute, time.Minute),
		newRateLimitBucket("interface:minute:user:"+interfaceID, "user", userID, "interface", config.InterfaceUserPerMinute, time.Minute),
		newRateLimitBucket("interface:minute:tenant:"+interfaceID, "tenant", tenantID, "interface", config.InterfaceTenantPerMinute, time.Minute),
	}

	switch category {
	case "ai":
		buckets = append(buckets,
			newRateLimitBucket("ai:minute:user", "user", userID, category, config.AIUserPerMinute, time.Minute),
			newRateLimitBucket("ai:minute:tenant", "tenant", tenantID, category, config.AITenantPerMinute, time.Minute),
			newRateLimitBucket("ai:day:user", "user", userID, category, config.AIUserPerDay, 24*time.Hour),
			newRateLimitBucket("ai:day:tenant", "tenant", tenantID, category, config.AITenantPerDay, 24*time.Hour),
		)
	case "publish":
		buckets = append(buckets,
			newRateLimitBucket("publish:minute:user", "user", userID, category, config.PublishUserPerMinute, time.Minute),
			newRateLimitBucket("publish:minute:tenant", "tenant", tenantID, category, config.PublishTenantPerMinute, time.Minute),
			newRateLimitBucket("publish:day:user", "user", userID, category, config.PublishUserPerDay, 24*time.Hour),
			newRateLimitBucket("publish:day:tenant", "tenant", tenantID, category, config.PublishTenantPerDay, 24*time.Hour),
		)
	case "browser_session":
		buckets = append(buckets,
			newRateLimitBucket("browser-session:minute:user", "user", userID, category, config.BrowserSessionUserPerMinute, time.Minute),
			newRateLimitBucket("browser-session:minute:tenant", "tenant", tenantID, category, config.BrowserSessionTenantPerMinute, time.Minute),
			newRateLimitBucket("browser-session:day:user", "user", userID, category, config.BrowserSessionUserPerDay, 24*time.Hour),
			newRateLimitBucket("browser-session:day:tenant", "tenant", tenantID, category, config.BrowserSessionTenantPerDay, 24*time.Hour),
		)
	}

	return enabledRateLimitBuckets(buckets), nil
}

func newRateLimitBucket(name, scope, identifier, category string, limit int64, window time.Duration) rateLimitBucket {
	return rateLimitBucket{
		Name:       name,
		Scope:      scope,
		Identifier: identifier,
		Category:   category,
		Limit:      limit,
		Window:     window,
	}
}

func enabledRateLimitBuckets(buckets []rateLimitBucket) []rateLimitBucket {
	enabled := buckets[:0]
	for _, bucket := range buckets {
		if bucket.Limit > 0 && bucket.Window > 0 {
			enabled = append(enabled, bucket)
		}
	}
	return enabled
}

func rateLimitCategory(method, route string) string {
	route = strings.ToLower(route)
	switch {
	case strings.Contains(route, "/ai/"):
		return "ai"
	case method == http.MethodPost && strings.HasSuffix(route, "/publish"):
		return "publish"
	case method == http.MethodPost && strings.Contains(route, "/browser-session"):
		return "browser_session"
	default:
		return "general"
	}
}

func checkRateLimitBuckets(ctx context.Context, client *redis.Client, prefix string, guard *redisdegrade.Guard, buckets []rateLimitBucket) (rateLimitResult, error) {
	var selected rateLimitResult
	for _, bucket := range buckets {
		current, ttl, err := incrementRateLimitBucket(ctx, client, guard, rateLimitRedisKey(prefix, bucket), bucket.Window)
		if err != nil {
			return rateLimitResult{}, err
		}

		remaining := max(bucket.Limit-current, 0)

		result := rateLimitResult{
			Bucket:     bucket,
			Current:    current,
			Remaining:  remaining,
			RetryAfter: ttl,
			Exceeded:   current > bucket.Limit,
		}
		if result.Exceeded {
			// A known exceeded bucket must not be downgraded by later Redis degradation.
			return result, nil
		}
		if selected.Bucket.Limit == 0 || result.Remaining < selected.Remaining {
			selected = result
		}
	}
	return selected, nil
}

func incrementRateLimitBucket(ctx context.Context, client *redis.Client, guard *redisdegrade.Guard, key string, window time.Duration) (int64, time.Duration, error) {
	raw, err := redisdegrade.CallWork(guard, "rate_limit", func() (any, error) {
		return client.Eval(ctx, redisRateLimitScript, []string{key}, window.Milliseconds()).Result()
	})
	if err != nil {
		return 0, 0, err
	}

	values, ok := raw.([]any)
	if !ok || len(values) != 2 {
		return 0, 0, fmt.Errorf("unexpected redis rate limit response")
	}

	current, ok := redisInt64(values[0])
	if !ok {
		return 0, 0, fmt.Errorf("unexpected redis rate limit count")
	}
	ttlMillis, ok := redisInt64(values[1])
	if !ok {
		return 0, 0, fmt.Errorf("unexpected redis rate limit ttl")
	}
	if ttlMillis < 0 {
		ttlMillis = window.Milliseconds()
	}

	return current, time.Duration(ttlMillis) * time.Millisecond, nil
}

func redisInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func rateLimitRedisKey(prefix string, bucket rateLimitBucket) string {
	return strings.Join([]string{
		strings.TrimRight(prefix, ":"),
		sanitizeRateLimitKeyPart(bucket.Scope),
		sanitizeRateLimitKeyPart(bucket.Identifier),
		sanitizeRateLimitKeyPart(bucket.Category),
		sanitizeRateLimitKeyPart(bucket.Name),
	}, ":")
}

func sanitizeRateLimitKeyPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ':' || r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "unknown"
	}
	return sanitized
}

func writeRateLimitHeaders(c echo.Context, result rateLimitResult) {
	if result.Bucket.Limit == 0 {
		return
	}

	headers := c.Response().Header()
	headers.Set("X-RateLimit-Limit", strconv.FormatInt(result.Bucket.Limit, 10))
	headers.Set("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(result.RetryAfter).Unix(), 10))
	headers.Set("X-RateLimit-Bucket", result.Bucket.Category)
	if result.Exceeded {
		headers.Set("Retry-After", strconv.FormatInt(resetAfterSeconds(result.RetryAfter), 10))
	}
}

func resetAfterSeconds(duration time.Duration) int64 {
	seconds := int64(math.Ceil(duration.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func envFlagEnabledDefault(name string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
