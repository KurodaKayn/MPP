package dashboard_test

import (
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
	"testing"
)

func TestGetProjectPublications(t *testing.T) {
	db := setupTestDB()
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
	assert.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)

	// Owner can see it
	resOwner, errOwner := s.GetProjectPublications(p.ID, &u1.ID, false)
	assert.NoError(t, errOwner)
	assert.Equal(t, p.ID, resOwner.ProjectID)

	// Collaborator can see it
	resCollaborator, errCollaborator := s.GetProjectPublications(p.ID, &collaborator.ID, false)
	assert.NoError(t, errCollaborator)
	assert.Equal(t, p.ID, resCollaborator.ProjectID)

	// Stranger gets Forbidden
	_, errStranger := s.GetProjectPublications(p.ID, &u2.ID, false)
	assert.ErrorIs(t, errStranger, services.ErrForbidden)
}
