package app

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNewRuntimeAllowsOptionalRedisForAPI(t *testing.T) {
	clearRuntimeEnv(t)
	database := newRuntimeTestDB(t)

	runtime, err := NewRuntime(context.Background(), RuntimeWiringConfig{
		Mode: RuntimeModeAPI,
		RuntimeConfig: RuntimeConfig{
			ProcessRole: ProcessRoleAPI,
		},
		JWTSecret: "jwt-secret",
		SQLDB:     database,
	})
	if err != nil {
		t.Fatalf("expected runtime wiring: %v", err)
	}

	if runtime.RedisClient != nil {
		t.Fatal("expected redis to remain optional for API runtime")
	}
	if runtime.DashboardService == nil {
		t.Fatal("expected dashboard service to be wired")
	}
	if runtime.BrowserSessionService == nil {
		t.Fatal("expected browser session service to be wired")
	}
	if runtime.CollabDocumentService == nil {
		t.Fatal("expected collab document service to be wired when JWT secret is provided")
	}
	if runtime.EmailService == nil {
		t.Fatal("expected email service to be wired")
	}
}

func TestNewRuntimeRequiresRedisForPublishWorker(t *testing.T) {
	clearRuntimeEnv(t)
	database := newRuntimeTestDB(t)

	_, err := NewRuntime(context.Background(), RuntimeWiringConfig{
		Mode: RuntimeModePublishWorker,
		RuntimeConfig: RuntimeConfig{
			ProcessRole:  ProcessRoleWorker,
			RequireRedis: true,
		},
		SQLDB: database,
	})
	if err == nil {
		t.Fatal("expected missing redis to fail publish worker runtime")
	}
	if !strings.Contains(err.Error(), "REDIS_ADDR must be set for publish-worker") {
		t.Fatalf("expected publish-worker redis error, got %v", err)
	}
}

func TestNewRuntimeBuildsRedisRoleClientsWhenConfigured(t *testing.T) {
	clearRuntimeEnv(t)
	database := newRuntimeTestDB(t)
	redisServer := miniredis.RunT(t)
	t.Setenv("REDIS_ADDR", redisServer.Addr())

	runtime, err := NewRuntime(context.Background(), RuntimeWiringConfig{
		Mode: RuntimeModeAPI,
		RuntimeConfig: RuntimeConfig{
			ProcessRole: ProcessRoleAPI,
		},
		JWTSecret: "jwt-secret",
		SQLDB:     database,
	})
	if err != nil {
		t.Fatalf("expected runtime wiring with redis: %v", err)
	}
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	if runtime.RedisClient == nil || runtime.RedisCoordination == nil || runtime.RedisCache == nil || runtime.RedisQueue == nil || runtime.RedisSessionContinuity == nil {
		t.Fatal("expected all redis role clients to be wired")
	}
	if runtime.RedisCoordination.Options().DialTimeout.String() != "500ms" {
		t.Fatalf("expected coordination client baseline, got %s", runtime.RedisCoordination.Options().DialTimeout)
	}
}

func newRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("expected test database: %v", err)
	}
	return database
}

func clearRuntimeEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"REDIS_ADDR",
		"REDIS_PASSWORD",
		"REDIS_DB",
		"REDIS_TLS",
		"REDIS_POOL_SIZE",
		"REDIS_MIN_IDLE_CONNS",
		"REDIS_MAX_IDLE_CONNS",
		"REDIS_CONN_MAX_IDLE_TIME",
		"REDIS_CONN_MAX_LIFETIME",
		"OBJECT_STORAGE_PROVIDER",
		"MEDIA_UPLOAD_URL_TTL",
		"MEDIA_DOWNLOAD_URL_TTL",
		"EVENT_ARCHIVE_ENABLED",
		"EVENT_ARCHIVE_INTERVAL",
		"EVENT_ARCHIVE_BATCH_SIZE",
		"PUBLISH_EVENT_RETENTION_DAYS",
		"EXTENSION_EXECUTION_EVENT_RETENTION_DAYS",
		"PROJECT_ACTIVITY_RETENTION_DAYS",
		"WORKSPACE_ACTIVITY_RETENTION_DAYS",
		"BROWSER_SESSION_HISTORY_RETENTION_DAYS",
		"SMTP_HOST",
		"SMTP_PORT",
		"SMTP_FROM",
		"SMTP_PASSWORD",
		"BROWSER_WORKER_URL",
		CollabTokenSecretEnv,
		CollabInternalURLEnv,
		CollabWebsocketURLBaseEnv,
	} {
		t.Setenv(name, "")
	}
}
