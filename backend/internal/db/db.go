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

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var DB *gorm.DB
var DefaultRouter *Router

//go:embed seed/seed_data.sql
var seedDataSQL string

// Stable app-specific key for the Postgres transaction advisory lock around migrations.
const migrationAdvisoryLockKey = 776770001
const devFallbackPasswordHash = "$2a$10$JuGX0AMl3DS3eGm/yRvY2OZLm4QuTuoIgRT4ucmVs/BCwoPYARN4C" //nolint:gosec // Development fallback is a bcrypt hash, not a plaintext password.
const disabledPasswordHash = "legacy-password-reset-required"

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

	if err := migrate(database); err != nil {
		log.Fatal("Failed to migrate database:", err)
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
	}), &gorm.Config{})
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

func migrate(database *gorm.DB) error {
	return withMigrationLock(database, func(migrationDB *gorm.DB) error {
		if err := prepareUserEmailMigration(migrationDB); err != nil {
			return err
		}
		if err := prepareUserPasswordHashMigration(migrationDB); err != nil {
			return err
		}
		if err := preparePlatformAccountWorkspaceMigration(migrationDB); err != nil {
			return err
		}
		if err := migrationDB.AutoMigrate(
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
			&models.ExtensionExecutionEvent{},
		); err != nil {
			return err
		}

		if err := migratePublicationStatuses(migrationDB); err != nil {
			return err
		}

		if err := backfillPersonalWorkspaces(migrationDB); err != nil {
			return err
		}
		if err := backfillPlatformAccountWorkspaces(migrationDB); err != nil {
			return err
		}

		// Redis owns normal active-session locking; this index is the atomic fallback when Redis is disabled.
		if err := migrationDB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS ux_remote_browser_sessions_active_user_platform
		ON remote_browser_sessions (user_id, platform)
		WHERE status IN ('pending', 'ready', 'login_detected', 'capturing')
	`).Error; err != nil {
			return err
		}
		if migrationDB.Name() == "postgres" {
			if err := migrationDB.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS ux_platform_accounts_workspace_platform_remote
				ON platform_accounts (workspace_id, platform, platform_user_id)
				WHERE platform_user_id IS NOT NULL AND platform_user_id <> ''
			`).Error; err != nil {
				return err
			}
			if err := migrationDB.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS ux_platform_accounts_workspace_platform_display
				ON platform_accounts (workspace_id, platform, display_name)
				WHERE (platform_user_id IS NULL OR platform_user_id = '') AND display_name IS NOT NULL AND display_name <> ''
			`).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func migratePublicationStatuses(database *gorm.DB) error {
	if !database.Migrator().HasTable(&models.ProjectPlatformPublication{}) {
		return nil
	}

	statusMap := map[string]string{
		"pending":   models.PublicationStatusDraft,
		"adapted":   models.PublicationStatusDraft,
		"published": models.PublicationStatusSucceeded,
		"disabled":  models.PublicationStatusCancelled,
	}
	for oldStatus, newStatus := range statusMap {
		if err := database.Model(&models.ProjectPlatformPublication{}).
			Where("status = ?", oldStatus).
			Update("status", newStatus).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillPersonalWorkspaces(database *gorm.DB) error {
	var users []models.User
	return database.FindInBatches(&users, 100, func(tx *gorm.DB, _ int) error {
		for _, user := range users {
			workspaceID := models.PersonalWorkspaceID(user.ID)
			workspace := models.Workspace{
				ID:          workspaceID,
				OwnerUserID: user.ID,
				Name:        models.PersonalWorkspaceName,
				Slug:        models.PersonalWorkspaceSlug(user.ID),
				Status:      models.WorkspaceStatusActive,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).Create(&workspace).Error; err != nil {
				return err
			}

			now := time.Now()
			member := models.WorkspaceMember{
				WorkspaceID: workspaceID,
				UserID:      user.ID,
				Role:        models.WorkspaceRoleOwner,
				JoinedAt:    &now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
				DoNothing: true,
			}).Create(&member).Error; err != nil {
				return err
			}

			if err := tx.Model(&models.Project{}).
				Where("user_id = ? AND workspace_id IS NULL", user.ID).
				Update("workspace_id", workspaceID).Error; err != nil {
				return err
			}
		}
		return nil
	}).Error
}

func preparePlatformAccountWorkspaceMigration(database *gorm.DB) error {
	if database.Name() != "postgres" {
		return nil
	}
	if !database.Migrator().HasTable(&models.PlatformAccount{}) {
		return nil
	}
	return database.Exec(`DROP INDEX IF EXISTS idx_platform_accounts_user_platform`).Error
}

func backfillPlatformAccountWorkspaces(database *gorm.DB) error {
	if !database.Migrator().HasTable(&models.PlatformAccount{}) {
		return nil
	}
	var accounts []models.PlatformAccount
	return database.FindInBatches(&accounts, 100, func(tx *gorm.DB, _ int) error {
		for _, account := range accounts {
			updates := map[string]any{}
			if account.WorkspaceID == nil || *account.WorkspaceID == uuid.Nil {
				workspaceID := models.PersonalWorkspaceID(account.UserID)
				updates["workspace_id"] = workspaceID
			}
			if account.OwnerUserID == nil {
				updates["owner_user_id"] = account.UserID
			}
			if account.ConnectedByUserID == nil {
				updates["connected_by_user_id"] = account.UserID
			}
			if strings.TrimSpace(account.DisplayName) == "" {
				displayName := account.Username
				if strings.TrimSpace(displayName) == "" {
					displayName = account.Platform
				}
				updates["display_name"] = displayName
			}
			if strings.TrimSpace(account.ShareScope) == "" {
				updates["share_scope"] = models.PlatformAccountSharePrivate
			}
			if strings.TrimSpace(account.HealthStatus) == "" {
				updates["health_status"] = healthStatusForPlatformAccountStatus(account.Status)
			}
			if strings.TrimSpace(account.CredentialSecretRef) == "" {
				updates["credential_secret_ref"] = "platform-account:" + account.ID.String()
			}
			if len(updates) > 0 {
				if err := tx.Model(&models.PlatformAccount{}).Where("id = ?", account.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}).Error
}

func healthStatusForPlatformAccountStatus(status string) string {
	switch status {
	case models.PlatformAccountStatusConnected:
		return models.PlatformAccountHealthHealthy
	case models.PlatformAccountStatusFailed:
		return models.PlatformAccountHealthFailed
	case models.PlatformAccountStatusNeedsReauth:
		return models.PlatformAccountHealthNeedsReauth
	default:
		return models.PlatformAccountHealthUnknown
	}
}

func prepareUserEmailMigration(database *gorm.DB) error {
	if database.Name() != "postgres" {
		return nil
	}
	if !database.Migrator().HasTable(&models.User{}) {
		return nil
	}

	if !database.Migrator().HasColumn(&models.User{}, "email") {
		if err := database.Exec(`ALTER TABLE users ADD COLUMN email text`).Error; err != nil {
			return err
		}
	}

	if err := database.Exec(`
		UPDATE users
		SET email = username || '-' || substring(id::text, 1, 8) || '@local.invalid'
		WHERE email IS NULL OR email = ''
	`).Error; err != nil {
		return err
	}
	if err := database.Exec(`ALTER TABLE users ALTER COLUMN email SET NOT NULL`).Error; err != nil {
		return err
	}
	return database.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email)`).Error
}

func prepareUserPasswordHashMigration(database *gorm.DB) error {
	if database.Name() != "postgres" {
		return nil
	}
	if !database.Migrator().HasTable(&models.User{}) {
		return nil
	}

	if !database.Migrator().HasColumn(&models.User{}, "password_hash") {
		if err := database.Exec(`ALTER TABLE users ADD COLUMN password_hash text`).Error; err != nil {
			return err
		}
	}

	passwordHash := disabledPasswordHash
	if devSeedEnabled() {
		passwordHash = devFallbackPasswordHash
	}

	if err := database.Exec(`
		UPDATE users
		SET password_hash = ?
		WHERE password_hash IS NULL OR password_hash = ''
	`, passwordHash).Error; err != nil {
		return err
	}
	return database.Exec(`ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL`).Error
}

func withMigrationLock(database *gorm.DB, run func(*gorm.DB) error) error {
	if database.Name() != "postgres" {
		return run(database)
	}

	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationAdvisoryLockKey).Error; err != nil {
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
