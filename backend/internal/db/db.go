package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var DB *gorm.DB
var DefaultRouter *Router

//go:embed seed/seed_data.sql
var seedDataSQL string

const schemaAdvisoryLockKey = 776770001

const (
	dbMaxOpenConnsEnv    = "DB_MAX_OPEN_CONNS"
	dbMaxIdleConnsEnv    = "DB_MAX_IDLE_CONNS"
	dbConnMaxLifetimeEnv = "DB_CONN_MAX_LIFETIME"
	dbConnMaxIdleTimeEnv = "DB_CONN_MAX_IDLE_TIME"
	dbSSLModeEnv         = "DB_SSLMODE"
	dbSSLRootCertEnv     = "DB_SSLROOTCERT"
	dbReaderHostEnv      = "DB_READER_HOST"
	dbReaderUserEnv      = "DB_READER_USER"
	dbReaderPasswordEnv  = "DB_READER_PASSWORD" //nolint:gosec // This is an environment variable name, not a password value.
	dbReaderNameEnv      = "DB_READER_NAME"
	dbReaderPortEnv      = "DB_READER_PORT"
	dbReaderSSLModeEnv   = "DB_READER_SSLMODE"
	dbReaderSSLRootEnv   = "DB_READER_SSLROOTCERT"
	dbReaderMaxLagEnv    = "DB_READER_MAX_REPLICA_LAG"
	dbReaderLagCheckEnv  = "DB_READER_LAG_CHECK_INTERVAL"
	defaultMaxOpenConns  = 10
	defaultMaxIdleConns  = 5
	defaultConnMaxLife   = 30 * time.Minute
	defaultConnMaxIdle   = 5 * time.Minute
	defaultDBSSLMode     = "disable"
	defaultLagCheck      = 5 * time.Second
)

type connectionPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func InitDB() {
	dsn, err := postgresDSNFromEnv()
	if err != nil {
		log.Fatal("Failed to configure database connection:", err)
	}
	database, err := openPostgresDatabase(dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	DB = database
	DefaultRouter = NewRouter(database)
	fmt.Println("Database connection established")

	if err := syncSchema(database); err != nil {
		log.Fatal("Failed to initialize database schema:", err)
	}

	reader, err := optionalPostgresReadReplicaFromEnv()
	if err != nil {
		log.Fatal("Failed to connect to database read replica:", err)
	}
	if reader != nil {
		options := []RouterOption{WithReader(reader)}
		lagMonitor, err := optionalReplicaLagMonitorFromEnv(reader)
		if err != nil {
			log.Fatal("Failed to configure database read replica lag monitor:", err)
		}
		if lagMonitor != nil {
			options = append(options, WithReplicaLagChecker(lagMonitor))
		}
		DefaultRouter = NewRouter(database, options...)
		fmt.Println("Database read replica connection established")
	}

	if devSeedEnabled() {
		if err := seed(database); err != nil {
			log.Fatal("Failed to seed database:", err)
		}
	}
}

func openPostgresDatabase(dsn string) (*gorm.DB, error) {
	database, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	if err := configureConnectionPool(database); err != nil {
		return nil, err
	}
	return database, nil
}

func optionalPostgresReadReplicaFromEnv() (*gorm.DB, error) {
	dsn, enabled, err := postgresReadReplicaDSNFromEnv()
	if err != nil || !enabled {
		return nil, err
	}
	return openPostgresDatabase(dsn)
}

func optionalReplicaLagMonitorFromEnv(reader *gorm.DB) (*ReplicaLagMonitor, error) {
	config, enabled, err := replicaLagMonitorConfigFromEnv()
	if err != nil || !enabled {
		return nil, err
	}
	return NewPostgresReplicaLagMonitor(reader, config), nil
}

func postgresDSNFromEnv() (string, error) {
	sslMode, err := postgresSSLModeFromEnv()
	if err != nil {
		return "", err
	}
	return postgresDSN(
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
		sslMode,
		strings.TrimSpace(os.Getenv(dbSSLRootCertEnv)),
	), nil
}

func postgresReadReplicaDSNFromEnv() (string, bool, error) {
	host := strings.TrimSpace(os.Getenv(dbReaderHostEnv))
	if host == "" {
		return "", false, nil
	}

	sslMode, err := postgresReadReplicaSSLModeFromEnv()
	if err != nil {
		return "", false, err
	}
	return postgresDSN(
		host,
		envWithFallback(dbReaderUserEnv, "DB_USER"),
		envWithFallback(dbReaderPasswordEnv, "DB_PASSWORD"),
		envWithFallback(dbReaderNameEnv, "DB_NAME"),
		envWithFallback(dbReaderPortEnv, "DB_PORT"),
		sslMode,
		strings.TrimSpace(envWithFallback(dbReaderSSLRootEnv, dbSSLRootCertEnv)),
	), true, nil
}

func postgresDSN(host, user, password, name, port, sslMode, sslRootCert string) string {
	parts := []string{
		postgresDSNParam("host", host),
		postgresDSNParam("user", user),
		postgresDSNParam("password", password),
		postgresDSNParam("dbname", name),
		postgresDSNParam("port", port),
		postgresDSNParam("sslmode", sslMode),
		postgresDSNRawParam("TimeZone", "Asia/Shanghai"),
	}
	if sslRootCert != "" {
		parts = append(parts, postgresDSNParam("sslrootcert", sslRootCert))
	}
	return strings.Join(parts, " ")
}

func postgresSSLModeFromEnv() (string, error) {
	return postgresSSLModeFromNamedEnv(dbSSLModeEnv, defaultDBSSLMode)
}

func postgresReadReplicaSSLModeFromEnv() (string, error) {
	if strings.TrimSpace(os.Getenv(dbReaderSSLModeEnv)) != "" {
		return postgresSSLModeFromNamedEnv(dbReaderSSLModeEnv, defaultDBSSLMode)
	}
	return postgresSSLModeFromEnv()
}

func postgresSSLModeFromNamedEnv(name string, fallback string) (string, error) {
	sslMode := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if sslMode == "" {
		return fallback, nil
	}
	switch sslMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return sslMode, nil
	default:
		return "", fmt.Errorf("invalid %s %q: expected disable, allow, prefer, require, verify-ca, or verify-full", name, sslMode)
	}
}

func envWithFallback(name string, fallbackName string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return os.Getenv(fallbackName)
}

func postgresDSNParam(key string, value string) string {
	return key + "=" + quotePostgresDSNValue(value)
}

func postgresDSNRawParam(key string, value string) string {
	return key + "=" + value
}

func quotePostgresDSNValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

func configureConnectionPool(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}

	config, err := connectionPoolConfigFromEnv()
	if err != nil {
		return err
	}
	applyConnectionPool(sqlDB, config)
	return nil
}

func connectionPoolConfigFromEnv() (connectionPoolConfig, error) {
	maxOpenConns, err := nonNegativeIntFromEnv(dbMaxOpenConnsEnv, defaultMaxOpenConns)
	if err != nil {
		return connectionPoolConfig{}, err
	}
	maxIdleConns, err := nonNegativeIntFromEnv(dbMaxIdleConnsEnv, defaultMaxIdleConns)
	if err != nil {
		return connectionPoolConfig{}, err
	}

	connMaxLifetime, err := durationFromEnv(dbConnMaxLifetimeEnv, defaultConnMaxLife)
	if err != nil {
		return connectionPoolConfig{}, err
	}
	connMaxIdleTime, err := durationFromEnv(dbConnMaxIdleTimeEnv, defaultConnMaxIdle)
	if err != nil {
		return connectionPoolConfig{}, err
	}

	return connectionPoolConfig{
		MaxOpenConns:    maxOpenConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
		ConnMaxIdleTime: connMaxIdleTime,
	}, nil
}

func applyConnectionPool(database *sql.DB, config connectionPoolConfig) {
	database.SetMaxOpenConns(config.MaxOpenConns)
	database.SetMaxIdleConns(config.MaxIdleConns)
	database.SetConnMaxLifetime(config.ConnMaxLifetime)
	database.SetConnMaxIdleTime(config.ConnMaxIdleTime)
}

func replicaLagMonitorConfigFromEnv() (ReplicaLagMonitorConfig, bool, error) {
	maxLag, err := durationFromEnv(dbReaderMaxLagEnv, 0)
	if err != nil {
		return ReplicaLagMonitorConfig{}, false, err
	}
	if maxLag <= 0 {
		return ReplicaLagMonitorConfig{}, false, nil
	}

	checkInterval, err := durationFromEnv(dbReaderLagCheckEnv, defaultLagCheck)
	if err != nil {
		return ReplicaLagMonitorConfig{}, false, err
	}
	if checkInterval <= 0 {
		checkInterval = defaultLagCheck
	}

	return ReplicaLagMonitorConfig{
		MaxLag:        maxLag,
		CheckInterval: checkInterval,
	}, true, nil
}

func nonNegativeIntFromEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s: must be non-negative", name)
	}
	return value, nil
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s: must be non-negative", name)
	}
	return value, nil
}

func syncSchema(database *gorm.DB) error {
	return withSchemaLock(database, syncSchemaUnlocked)
}

func syncSchemaUnlocked(database *gorm.DB) error {
	if err := ensureMonthlyEventPartitions(database, time.Now().UTC()); err != nil {
		return err
	}
	if err := ensureCollabUpdateBatchHashPartitions(database); err != nil {
		return err
	}

	if err := database.AutoMigrate(
		&models.User{},
		&models.Workspace{},
		&models.WorkspaceMember{},
		&models.WorkspaceInvite{},
		&models.WorkspaceActivity{},
		&models.WorkspaceDashboardStats{},
		&models.Notification{},
		&models.PlatformAccount{},
		&models.PlatformAccountGrant{},
		&models.Project{},
		&models.ContentTemplate{},
		&models.BrandProfile{},
		&models.ProjectCollaborator{},
		&models.ProjectActivity{},
		&models.ProjectComment{},
		&models.ProjectVersion{},
		&models.ProjectShareLink{},
		&models.MediaAsset{},
		&models.MediaAssetUsage{},
		&models.ProjectPlatformPublication{},
		&models.ProjectListSummary{},
		&models.ScheduledPublication{},
		&models.PublishAttempt{},
		&models.RemoteBrowserSession{},
		&models.PublishEvent{},
		&models.AIContextSnapshot{},
		&models.AIGrowthOptimizationRun{},
		&models.AIProposal{},
		&models.AIDraftingSession{},
		&models.AIDraftingMessage{},
		&models.AIToolCall{},
		&models.AIDraftingSessionSummary{},
		&models.AISessionEvent{},
		&models.AIUsageRecord{},
		&models.WorkspaceQuotaAggregate{},
		&models.OutboxEvent{},
		&models.CollabDocument{},
		&models.CollabDocumentCollaborator{},
		&models.CollabDocumentState{},
		&models.CollabDocumentUpdateBatch{},
		&models.ExtensionCallbackToken{},
		&models.ExtensionExecutionEventClaim{},
		&models.ExtensionExecutionEvent{},
	); err != nil {
		return err
	}
	if err := backfillExtensionExecutionEventClaims(database); err != nil {
		return err
	}

	if database.Name() != "postgres" {
		// Redis owns normal active-session locking. Non-PostgreSQL local/test databases keep
		// a partial unique index as the no-Redis fallback; PostgreSQL uses a scoped advisory
		// transaction lock because partitioned unique indexes must include the partition key.
		if err := database.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS ux_remote_browser_sessions_active_user_platform
			ON remote_browser_sessions (user_id, platform)
			WHERE status IN ('pending', 'ready', 'login_detected', 'capturing')
		`).Error; err != nil {
			return err
		}
	}
	if database.Name() == "postgres" {
		if err := database.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS ux_platform_accounts_workspace_platform_remote
			ON platform_accounts (workspace_id, platform, platform_user_id)
			WHERE platform_user_id IS NOT NULL AND platform_user_id <> ''
		`).Error; err != nil {
			return err
		}
		if err := database.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS ux_platform_accounts_workspace_platform_display
			ON platform_accounts (workspace_id, platform, display_name)
			WHERE (platform_user_id IS NULL OR platform_user_id = '') AND display_name IS NOT NULL AND display_name <> ''
		`).Error; err != nil {
			return err
		}
	}
	return nil
}

func withSchemaLock(database *gorm.DB, run func(*gorm.DB) error) error {
	if database.Name() != "postgres" {
		return run(database)
	}

	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", schemaAdvisoryLockKey).Error; err != nil {
			return err
		}
		return run(tx)
	})
}

func seed(database *gorm.DB) error {
	if strings.TrimSpace(seedDataSQL) == "" {
		return nil
	}

	return database.Exec(seedDataSQL).Error
}

func devSeedEnabled() bool {
	localEnv := isLocalEnvironment(os.Getenv("APP_ENV")) || isLocalEnvironment(os.Getenv("NODE_ENV"))
	return localEnv && envFlagEnabled("ENABLE_DEV_SEED")
}

func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func isLocalEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "dev", "development":
		return true
	default:
		return false
	}
}
