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

	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

//go:embed seed/seed_data.sql
var seedDataSQL string

// Stable app-specific key for the Postgres transaction advisory lock around migrations.
const migrationAdvisoryLockKey = 776770001
const devFallbackPasswordHash = "$2a$10$JuGX0AMl3DS3eGm/yRvY2OZLm4QuTuoIgRT4ucmVs/BCwoPYARN4C"
const disabledPasswordHash = "legacy-password-reset-required"

const (
	dbMaxOpenConnsEnv    = "DB_MAX_OPEN_CONNS"
	dbMaxIdleConnsEnv    = "DB_MAX_IDLE_CONNS"
	dbConnMaxLifetimeEnv = "DB_CONN_MAX_LIFETIME"
	dbConnMaxIdleTimeEnv = "DB_CONN_MAX_IDLE_TIME"
	defaultMaxOpenConns  = 10
	defaultMaxIdleConns  = 5
	defaultConnMaxLife   = 30 * time.Minute
	defaultConnMaxIdle   = 5 * time.Minute
)

type connectionPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func InitDB() {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Shanghai",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	if err := configureConnectionPool(database); err != nil {
		log.Fatal("Failed to configure database connection pool:", err)
	}

	DB = database
	fmt.Println("Database connection established")

	if err := migrate(database); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	if devSeedEnabled() {
		if err := seed(database); err != nil {
			log.Fatal("Failed to seed database:", err)
		}
	}
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
		if err := migrationDB.AutoMigrate(
			&models.User{},
			&models.PlatformAccount{},
			&models.Project{},
			&models.ProjectPlatformPublication{},
			&models.RemoteBrowserSession{},
			&models.CollabDocument{},
			&models.CollabDocumentCollaborator{},
			&models.CollabDocumentState{},
			&models.ExtensionCallbackToken{},
			&models.ExtensionExecutionEvent{},
		); err != nil {
			return err
		}

		// Redis owns normal active-session locking; this index is the atomic fallback when Redis is disabled.
		return migrationDB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS ux_remote_browser_sessions_active_user_platform
		ON remote_browser_sessions (user_id, platform)
		WHERE status IN ('pending', 'ready', 'login_detected', 'capturing')
	`).Error
	})
}

func prepareUserEmailMigration(database *gorm.DB) error {
	if database.Dialector.Name() != "postgres" {
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
	if database.Dialector.Name() != "postgres" {
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
	if database.Dialector.Name() != "postgres" {
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
