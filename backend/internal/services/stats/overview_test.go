package stats_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestGetStats(t *testing.T) {
	db := testsupport.SetupTestDB()
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
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)

	// Test Personal scope (u1)
	statsScoped, errScoped := s.GetStats(&u1.ID)
	require.NoError(t, errScoped)
	assert.Equal(t, int64(1), statsScoped.TotalUsers)
	assert.Equal(t, int64(1), statsScoped.TotalProjects)
	assert.Equal(t, int64(1), statsScoped.TotalPublishedPublications)
	assert.Equal(t, int64(0), statsScoped.TotalFailedPublications)
}

func TestGetStatsUsesReaderForEventualCounts(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	u1 := models.User{Username: "reader-user"}
	require.NoError(t, reader.Create(&u1).Error)
	project := models.Project{UserID: u1.ID, Title: "reader-project", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&project).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	var writerUsers int64
	require.NoError(t, writer.Model(&models.User{}).Count(&writerUsers).Error)
	require.Equal(t, int64(0), writerUsers)

	stats, err := s.GetStats(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
}

func TestGetStatsUsesWriterForScopedCounts(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	user := models.User{Username: "scoped-user"}
	require.NoError(t, writer.Create(&user).Error)
	currentProject := models.Project{UserID: user.ID, Title: "current-project", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, writer.Create(&currentProject).Error)
	require.NoError(t, writer.Create(&models.ProjectPlatformPublication{
		ProjectID: currentProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	staleProject := models.Project{UserID: user.ID, Title: "stale-reader-project", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&staleProject).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: staleProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: staleProject.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	stats, err := s.GetStats(&user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(0), stats.TotalFailedPublications)
}
