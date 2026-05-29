package services_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/dto"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// SQLite doesn't support gen_random_uuid(), so we create tables manually
	// avoiding the default constraints, and relying on our BeforeCreate hooks.
	db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE project_platform_publications (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		status TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		adapted_content TEXT NOT NULL DEFAULT '{}',
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		last_attempt_at DATETIME,
		published_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	return db
}

func TestGetStats(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	// Seed data
	u := models.User{Username: "test"}
	db.Create(&u)
	p := models.Project{UserID: u.ID, Title: "p1", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p.ID, Platform: "wechat", Status: models.PublicationStatusPublished})
	db.Create(&models.ProjectPlatformPublication{ProjectID: p.ID, Platform: "zhihu", Status: models.PublicationStatusFailed})
	db.Create(&models.ProjectPlatformPublication{ProjectID: p.ID, Platform: "bili", Status: models.PublicationStatusPending}) // should not count

	stats, err := s.GetStats()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)
}

func TestListProjects(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u := models.User{Username: "test"}
	db.Create(&u)
	p1 := models.Project{UserID: u.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished, CreatedAt: time.Now().Add(-1 * time.Hour)}
	p2 := models.Project{UserID: u.ID, Title: "p2", SourceContent: "c2", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	db.Create(&p1)
	db.Create(&p2)

	// Attach a publication to p1
	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished, PublishURL: "url1"})

	// Test pagination and sorting
	res, err := s.ListProjects(1, 10, "", "", "")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), res.Total)
	// Default sort is created_at desc, so p2 should be first
	items := res.Items.([]dto.ProjectListItem)
	assert.Equal(t, 2, len(items))

	// Test Status filter
	res2, err := s.ListProjects(1, 10, models.ProjectStatusPublished, "", "")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), res2.Total)
}

func TestGetProjectPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u := models.User{Username: "test"}
	db.Create(&u)
	p := models.Project{UserID: u.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished}
	db.Create(&p)

	// JSONB mock data
	configJSON := `{"title": "safe_title", "secret_token": "hidden"}`
	contentJSON := `{"summary": "safe_summary", "full_text": "huge..."}`

	pub := models.ProjectPlatformPublication{
		ProjectID:      p.ID,
		Platform:       "wechat",
		Status:         models.PublicationStatusPublished,
		Config:         datatypes.JSON(configJSON),
		AdaptedContent: datatypes.JSON(contentJSON),
	}
	db.Create(&pub)

	res, err := s.GetProjectPublications(p.ID)
	assert.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)
	assert.Len(t, res.Items, 1)

	item := res.Items[0]
	// Check Config whitelist
	assert.Equal(t, "safe_title", item.Config["title"])
	assert.NotContains(t, item.Config, "secret_token")

	// Check Adapted Content summary
	assert.Equal(t, "safe_summary", item.AdaptedContent["summary"])
	assert.NotContains(t, item.AdaptedContent, "full_text")

	// Test NotFound
	_, errNotFound := s.GetProjectPublications(uuid.New())
	assert.ErrorIs(t, errNotFound, gorm.ErrRecordNotFound)
}
