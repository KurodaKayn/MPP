package db

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

//go:embed seed/seed_data.sql
var seedDataSQL string

// Stable app-specific key for the Postgres transaction advisory lock around migrations.
const migrationAdvisoryLockKey = 776770001

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

func migrate(database *gorm.DB) error {
	return withMigrationLock(database, func(migrationDB *gorm.DB) error {
		if err := migrationDB.AutoMigrate(
			&models.User{},
			&models.PlatformAccount{},
			&models.Project{},
			&models.ProjectPlatformPublication{},
			&models.PlatformAccount{},
			&models.RemoteBrowserSession{},
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
