package project_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestProjectCollaboratorManagement(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	collaborator := models.User{Username: "collaborator", Email: "collaborator@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&collaborator).Error)
	require.NoError(t, db.Create(&stranger).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Shared project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	added, err := s.AddProjectCollaborator(project.ID, owner.ID, dto.AddProjectCollaboratorRequest{
		Email: "COLLABORATOR@example.com",
		Role:  models.ProjectRoleEditor,
	})
	require.NoError(t, err)
	require.Equal(t, collaborator.ID, added.UserID)
	require.Equal(t, collaborator.Email, added.Email)
	require.Equal(t, models.ProjectRoleEditor, added.Role)
	require.Equal(t, owner.ID, added.CreatedBy)

	list, err := s.ListProjectCollaborators(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, collaborator.ID, list.Items[0].UserID)

	updated, err := s.UpdateProjectCollaborator(project.ID, owner.ID, collaborator.ID, dto.UpdateProjectCollaboratorRequest{
		Role: models.ProjectRoleViewer,
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleViewer, updated.Role)

	_, err = s.AddProjectCollaborator(project.ID, owner.ID, dto.AddProjectCollaboratorRequest{
		UserID: owner.ID,
		Role:   models.ProjectRoleViewer,
	})
	require.ErrorIs(t, err, services.ErrInvalidProjectCollaborator)

	_, err = s.AddProjectCollaborator(project.ID, stranger.ID, dto.AddProjectCollaboratorRequest{
		UserID: collaborator.ID,
		Role:   models.ProjectRoleViewer,
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	require.NoError(t, s.RemoveProjectCollaborator(project.ID, owner.ID, collaborator.ID))
	list, err = s.ListProjectCollaborators(project.ID, owner.ID)
	require.NoError(t, err)
	require.Empty(t, list.Items)
}
