package project_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestGetProjectPublications(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "owner"}
	collaborator := models.User{Username: "collaborator", Email: "collaborator@example.com"}
	u2 := models.User{Username: "stranger"}
	db.Create(&u1)
	db.Create(&collaborator)
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
	db.Create(&models.ProjectCollaborator{
		ProjectID: p.ID,
		UserID:    collaborator.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: u1.ID,
	})

	// Admin can see it
	res, err := s.GetProjectPublications(p.ID, nil, false)
	require.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)

	// Owner can see it
	resOwner, errOwner := s.GetProjectPublications(p.ID, &u1.ID, false)
	require.NoError(t, errOwner)
	assert.Equal(t, p.ID, resOwner.ProjectID)

	// Collaborator can see it
	resCollaborator, errCollaborator := s.GetProjectPublications(p.ID, &collaborator.ID, false)
	require.NoError(t, errCollaborator)
	assert.Equal(t, p.ID, resCollaborator.ProjectID)

	// Stranger gets Forbidden
	_, errStranger := s.GetProjectPublications(p.ID, &u2.ID, false)
	require.ErrorIs(t, errStranger, services.ErrForbidden)
}

func TestGetProjectPublicationsUsesReaderForAdminDetail(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	user := models.User{Username: "reader-publications-owner", Email: "reader-publications-owner@example.com"}
	require.NoError(t, reader.Create(&user).Error)
	project := models.Project{UserID: user.ID, Title: "Reader publications", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&project).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID:  project.ID,
		Platform:   "wechat",
		Status:     models.PublicationStatusPublished,
		PublishURL: "https://example.test/reader",
	}).Error)

	res, err := s.GetProjectPublications(project.ID, nil, false)
	require.NoError(t, err)
	require.Equal(t, project.ID, res.ProjectID)
	require.Len(t, res.Items, 1)
	require.Equal(t, "wechat", res.Items[0].Platform)
	require.Equal(t, models.PublicationStatusPublished, res.Items[0].Status)
}

func TestGetProjectPublicationsUsesWriterForScopedDetail(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	owner := models.User{Username: "scoped-publications-owner", Email: "scoped-publications-owner@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	currentProject := models.Project{UserID: owner.ID, Title: "Current publications", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, writer.Create(&currentProject).Error)
	require.NoError(t, writer.Create(&models.ProjectPlatformPublication{
		ProjectID: currentProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	staleReaderProject := models.Project{
		ID:            currentProject.ID,
		UserID:        owner.ID,
		Title:         "Stale reader publications",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, reader.Create(&staleReaderProject).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: currentProject.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	res, err := s.GetProjectPublications(currentProject.ID, &owner.ID, false)
	require.NoError(t, err)
	require.Equal(t, currentProject.ID, res.ProjectID)
	require.Len(t, res.Items, 1)
	require.Equal(t, "wechat", res.Items[0].Platform)
	require.Equal(t, models.PublicationStatusPublished, res.Items[0].Status)
}

func TestGetProjectPublicationsUsesWriterForStickyAdminDetail(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))
	s := services.NewDashboardServiceWithRouter(writer, router).WithContext(stickyCtx)

	owner := models.User{Username: "sticky-publications-owner", Email: "sticky-publications-owner@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	currentProject := models.Project{UserID: owner.ID, Title: "Sticky publications", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, writer.Create(&currentProject).Error)
	require.NoError(t, writer.Create(&models.ProjectPlatformPublication{
		ProjectID: currentProject.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	staleReaderProject := models.Project{
		ID:            currentProject.ID,
		UserID:        owner.ID,
		Title:         "Stale sticky publications",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, reader.Create(&staleReaderProject).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: currentProject.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusFailed,
	}).Error)

	res, err := s.GetProjectPublications(currentProject.ID, nil, false)
	require.NoError(t, err)
	require.Equal(t, currentProject.ID, res.ProjectID)
	require.Len(t, res.Items, 1)
	require.Equal(t, "wechat", res.Items[0].Platform)
	require.Equal(t, models.PublicationStatusPublished, res.Items[0].Status)
}
