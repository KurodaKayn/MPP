package db

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

//go:embed seed/seed_data.sql
var seedDataSQL string

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
	return database.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.ProjectPlatformPublication{},
	)
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
