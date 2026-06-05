package project_test

import (
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
	"github.com/stretchr/testify/require"
)

func TestProjectCollaborationExperienceCommentsAndActivities(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	reviewer := models.User{Username: "reviewer", Email: "reviewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&reviewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Review draft",
		SourceContent: "<p>Draft</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    reviewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	comment, err := s.CreateProjectComment(project.ID, reviewer.ID, dto.CreateProjectCommentRequest{
		Body:       "Please tighten the introduction.",
		AnchorText: "introduction",
	})
	require.NoError(t, err)
	require.Equal(t, "Please tighten the introduction.", comment.Body)
	require.Equal(t, models.ProjectCommentStatusOpen, comment.Status)

	comments, err := s.ListProjectComments(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, comments.Items, 1)

	resolved, err := s.UpdateProjectComment(project.ID, owner.ID, comment.ID, dto.UpdateProjectCommentRequest{
		Status: models.ProjectCommentStatusResolved,
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectCommentStatusResolved, resolved.Status)
	require.NotNil(t, resolved.ResolvedAt)

	activities, err := s.ListProjectActivities(project.ID, owner.ID, 10)
	require.NoError(t, err)
	require.Len(t, activities.Items, 2)
	require.Equal(t, models.ProjectActivityCommentResolved, activities.Items[0].EventType)
	require.Equal(t, models.ProjectActivityCommentCreated, activities.Items[1].EventType)
}

func TestProjectVersionsRestoreSavedContent(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Draft v1",
		SourceContent: "<p>v1</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	_, err := s.SaveProjectContent(project.ID, owner.ID, dto.SaveProjectContentRequest{
		Title:         "Draft v2",
		SourceContent: "<p>v2</p>",
	})
	require.NoError(t, err)
	_, err = s.SaveProjectContent(project.ID, owner.ID, dto.SaveProjectContentRequest{
		Title:         "Draft v3",
		SourceContent: "<p>v3</p>",
	})
	require.NoError(t, err)

	versions, err := s.ListProjectVersions(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, versions.Items, 2)
	require.Equal(t, 2, versions.Items[0].VersionNumber)
	require.Equal(t, 1, versions.Items[1].VersionNumber)

	restored, err := s.RestoreProjectVersion(project.ID, owner.ID, versions.Items[1].ID)
	require.NoError(t, err)
	require.Equal(t, "Draft v2", restored.Project.Title)
	require.Equal(t, "<p>v2</p>", restored.Project.SourceContent)

	versions, err = s.ListProjectVersions(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, versions.Items, 3)
	require.Equal(t, "version_restore", versions.Items[0].Source)
}

func TestProjectShareLinksAreOwnerManaged(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Shared draft",
		SourceContent: "<p>Draft</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)

	_, err := s.CreateProjectShareLink(project.ID, editor.ID, dto.CreateProjectShareLinkRequest{
		Role: models.ProjectRoleViewer,
	}, "https://app.example.com")
	require.ErrorIs(t, err, services.ErrForbidden)

	link, err := s.CreateProjectShareLink(project.ID, owner.ID, dto.CreateProjectShareLinkRequest{
		Role: models.ProjectRoleViewer,
	}, "https://app.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, link.Token)
	require.Contains(t, link.URL, "/share/projects/")

	links, err := s.ListProjectShareLinks(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, links.Items, 1)
	require.Equal(t, models.ProjectShareLinkStatusActive, links.Items[0].Status)

	require.NoError(t, s.RevokeProjectShareLink(project.ID, owner.ID, link.ID))
	links, err = s.ListProjectShareLinks(project.ID, owner.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProjectShareLinkStatusRevoked, links.Items[0].Status)
}
