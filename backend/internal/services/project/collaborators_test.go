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

func TestOwnedProjectCollaboratorSummaries(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "summary-owner", Email: "summary-owner@example.com"}
	collaborator := models.User{Username: "summary-collaborator", Email: "summary-collaborator@example.com"}
	otherCollaborator := models.User{Username: "summary-other-collaborator", Email: "summary-other-collaborator@example.com"}
	otherOwner := models.User{Username: "summary-other-owner", Email: "summary-other-owner@example.com"}
	stranger := models.User{Username: "summary-stranger", Email: "summary-stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&collaborator).Error)
	require.NoError(t, db.Create(&otherCollaborator).Error)
	require.NoError(t, db.Create(&otherOwner).Error)
	require.NoError(t, db.Create(&stranger).Error)

	ownedProject := models.Project{
		UserID:        owner.ID,
		Title:         "Owned shared project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	unsharedOwnedProject := models.Project{
		UserID:        owner.ID,
		Title:         "Owned private project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	sharedWithOwnerProject := models.Project{
		UserID:        otherOwner.ID,
		Title:         "Shared with owner",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&ownedProject).Error)
	require.NoError(t, db.Create(&unsharedOwnedProject).Error)
	require.NoError(t, db.Create(&sharedWithOwnerProject).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: ownedProject.ID,
		UserID:    collaborator.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: ownedProject.ID,
		UserID:    otherCollaborator.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: sharedWithOwnerProject.ID,
		UserID:    owner.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: otherOwner.ID,
	}).Error)

	resp, err := s.ListOwnedProjectCollaboratorSummaries(owner.ID)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, ownedProject.ID, resp.Items[0].ProjectID)
	require.Equal(t, 2, resp.Items[0].CollaboratorCount)
	require.Len(t, resp.Items[0].Collaborators, 2)
	require.ElementsMatch(t, []string{collaborator.Email, otherCollaborator.Email}, []string{
		resp.Items[0].Collaborators[0].Email,
		resp.Items[0].Collaborators[1].Email,
	})

	strangerResp, err := s.ListOwnedProjectCollaboratorSummaries(stranger.ID)
	require.NoError(t, err)
	require.Empty(t, strangerResp.Items)

	_, err = s.ListOwnedProjectCollaboratorSummaries(models.User{}.ID)
	require.ErrorIs(t, err, services.ErrInvalidProject)
}
