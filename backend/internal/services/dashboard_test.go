package services_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
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

	u1 := models.User{Username: "test1"}
	u2 := models.User{Username: "test2"}
	db.Create(&u1)
	db.Create(&u2)
	
	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c", Status: models.ProjectStatusDraft}
	p2 := models.Project{UserID: u2.ID, Title: "p2", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p1)
	db.Create(&p2)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished})
	db.Create(&models.ProjectPlatformPublication{ProjectID: p2.ID, Platform: "zhihu", Status: models.PublicationStatusFailed})

	// Test Admin scope (nil scopeUserID)
	stats, err := s.GetStats(nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)

	// Test Personal scope (u1)
	statsScoped, errScoped := s.GetStats(&u1.ID)
	assert.NoError(t, errScoped)
	assert.Equal(t, int64(1), statsScoped.TotalUsers)
	assert.Equal(t, int64(1), statsScoped.TotalProjects)
	assert.Equal(t, int64(1), statsScoped.TotalPublishedPublications)
	assert.Equal(t, int64(0), statsScoped.TotalFailedPublications)
}

func TestListProjects(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test"}
	u2 := models.User{Username: "other"}
	db.Create(&u1)
	db.Create(&u2)
	
	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished, CreatedAt: time.Now().Add(-1 * time.Hour)}
	p2 := models.Project{UserID: u1.ID, Title: "p2", SourceContent: "c2", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	p3 := models.Project{UserID: u2.ID, Title: "p3", SourceContent: "c3", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	db.Create(&p1)
	db.Create(&p2)
	db.Create(&p3)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished, PublishURL: "url1"})

	// Test global admin pagination
	res, err := s.ListProjects(1, 10, "", "", "", nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), res.Total)
	
	// Test Personal scope (u1)
	resScoped, errScoped := s.ListProjects(1, 10, "", "", "", &u1.ID)
	assert.NoError(t, errScoped)
	assert.Equal(t, int64(2), resScoped.Total)
	items := resScoped.Items.([]dto.ProjectListItem)
	assert.Equal(t, 2, len(items))
	// Ensure p3 is not in list
	for _, item := range items {
		assert.NotEqual(t, p3.ID, item.ID)
	}
}

func TestGetProjectPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "owner"}
	u2 := models.User{Username: "stranger"}
	db.Create(&u1)
	db.Create(&u2)
	
	p := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished}
	db.Create(&p)

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

	// Admin can see it
	res, err := s.GetProjectPublications(p.ID, nil)
	assert.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)

	// Owner can see it
	resOwner, errOwner := s.GetProjectPublications(p.ID, &u1.ID)
	assert.NoError(t, errOwner)
	assert.Equal(t, p.ID, resOwner.ProjectID)
	
	// Stranger gets Forbidden
	_, errStranger := s.GetProjectPublications(p.ID, &u2.ID)
	assert.ErrorIs(t, errStranger, services.ErrForbidden)
}

