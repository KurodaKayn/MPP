package project_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
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
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusDraft,
		DraftStatus:    models.PublicationDraftStatusReady,
		ReviewStatus:   models.PublicationReviewStatusApproved,
		SyncRequired:   false,
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

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
	require.NoError(t, db.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND platform = ?", project.ID, "wechat").
		Updates(map[string]any{
			"draft_status":  models.PublicationDraftStatusReady,
			"review_status": models.PublicationReviewStatusApproved,
			"sync_required": false,
		}).Error)

	restored, err := s.RestoreProjectVersion(project.ID, owner.ID, versions.Items[1].ID)
	require.NoError(t, err)
	require.Equal(t, "Draft v2", restored.Project.Title)
	require.Equal(t, "<p>v2</p>", restored.Project.SourceContent)
	require.Len(t, restored.Project.Publications, 1)
	require.Equal(t, models.PublicationDraftStatusStale, restored.Project.Publications[0].DraftStatus)
	require.True(t, restored.Project.Publications[0].SyncRequired)

	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationDraftStatusStale, publication.DraftStatus)
	require.Equal(t, models.PublicationReviewStatusDraft, publication.ReviewStatus)
	require.True(t, publication.SyncRequired)

	versions, err = s.ListProjectVersions(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, versions.Items, 3)
	require.Equal(t, "version_restore", versions.Items[0].Source)
}

func TestProjectVersionRestoreDetachesCollabDocument(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "restore-owner", Email: "restore-owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)

	document := models.CollabDocument{
		OwnerUserID: owner.ID,
		Title:       "Realtime document",
		Status:      models.CollabDocumentStatusActive,
		CurrentSeq:  8,
	}
	require.NoError(t, db.Create(&document).Error)

	project := models.Project{
		UserID:           owner.ID,
		CollabDocumentID: &document.ID,
		Title:            "Draft v1",
		SourceContent:    "<p>v1</p>",
		Status:           models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectVersion{
		ProjectID:        project.ID,
		CreatedBy:        owner.ID,
		VersionNumber:    1,
		Title:            "Draft v1",
		SourceContent:    "<p>v1</p>",
		CollabDocumentID: &document.ID,
		CollabSeq:        8,
		Source:           "content_save",
	}).Error)

	versions, err := s.ListProjectVersions(project.ID, owner.ID)
	require.NoError(t, err)
	restored, err := s.RestoreProjectVersion(project.ID, owner.ID, versions.Items[0].ID)
	require.NoError(t, err)
	require.Nil(t, restored.Project.CollabDocumentID)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Nil(t, saved.CollabDocumentID)

	versions, err = s.ListProjectVersions(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, versions.Items, 2)
	require.Nil(t, versions.Items[0].CollabDocumentID)
	require.Equal(t, int64(0), versions.Items[0].CollabSeq)
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

func TestProjectShareLinkAcceptGrantsAccess(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "share-owner", Email: "share-owner@example.com"}
	viewer := models.User{Username: "share-viewer", Email: "share-viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Shared draft",
		SourceContent: "<p>Draft</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	link, err := s.CreateProjectShareLink(project.ID, owner.ID, dto.CreateProjectShareLinkRequest{
		Role: models.ProjectRoleViewer,
	}, "https://app.example.com")
	require.NoError(t, err)

	accepted, err := s.AcceptProjectShareLink(link.Token, viewer.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, accepted.Project.ID)
	require.Equal(t, models.ProjectRoleViewer, accepted.Role)

	collaborators, err := s.ListProjectCollaborators(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, collaborators.Items, 1)
	require.Equal(t, viewer.ID, collaborators.Items[0].UserID)
	require.Equal(t, models.ProjectRoleViewer, collaborators.Items[0].Role)
}

func TestProjectShareLinkAcceptPreservesStrongerAccess(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner-strong", Email: "owner-strong@example.com"}
	editor := models.User{Username: "editor-strong", Email: "editor-strong@example.com"}
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

	link, err := s.CreateProjectShareLink(project.ID, owner.ID, dto.CreateProjectShareLinkRequest{
		Role: models.ProjectRoleViewer,
	}, "https://app.example.com")
	require.NoError(t, err)

	accepted, err := s.AcceptProjectShareLink(link.Token, editor.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, accepted.Role)

	var collaborator models.ProjectCollaborator
	require.NoError(t, db.First(&collaborator, "project_id = ? AND user_id = ?", project.ID, editor.ID).Error)
	require.Equal(t, models.ProjectRoleEditor, collaborator.Role)
}

func TestProjectShareLinkAcceptRejectsRevokedAndExpiredTokens(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner-expired", Email: "owner-expired@example.com"}
	viewer := models.User{Username: "viewer-expired", Email: "viewer-expired@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Shared draft",
		SourceContent: "<p>Draft</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	revoked, err := s.CreateProjectShareLink(project.ID, owner.ID, dto.CreateProjectShareLinkRequest{
		Role: models.ProjectRoleViewer,
	}, "https://app.example.com")
	require.NoError(t, err)
	require.NoError(t, s.RevokeProjectShareLink(project.ID, owner.ID, revoked.ID))
	_, err = s.AcceptProjectShareLink(revoked.Token, viewer.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	expiresAt := time.Now().UTC().Add(-time.Minute)
	expiredToken := uuid.NewString()
	require.NoError(t, db.Create(&models.ProjectShareLink{
		ProjectID: project.ID,
		CreatedBy: owner.ID,
		TokenHash: hashProjectShareTokenForTest(expiredToken),
		Role:      models.ProjectRoleViewer,
		Status:    models.ProjectShareLinkStatusActive,
		ExpiresAt: &expiresAt,
	}).Error)
	_, err = s.AcceptProjectShareLink(expiredToken, viewer.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestProjectDetailMergesWorkspaceCollaboratorAndSharePermissionSources(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "perm-owner", Email: "perm-owner@example.com"}
	editor := models.User{Username: "perm-editor", Email: "perm-editor@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)

	workspace := models.Workspace{
		OwnerUserID: owner.ID,
		Name:        "Team",
		Slug:        "team",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      editor.ID,
		Role:        models.WorkspaceRoleViewer,
	}).Error)

	project := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Shared workspace draft",
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
	link, err := s.CreateProjectShareLink(project.ID, owner.ID, dto.CreateProjectShareLinkRequest{
		Role: models.ProjectRoleViewer,
	}, "https://app.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, link.Token)

	editorDetail, err := s.GetProject(project.ID, &editor.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []dto.ProjectPermissionSource{
		{Source: models.ProjectAccessSourceDirectShare, Role: models.ProjectRoleEditor},
		{Source: models.ProjectAccessSourceWorkspace, Role: models.ProjectRoleViewer},
	}, editorDetail.PermissionSources)

	ownerDetail, err := s.GetProject(project.ID, &owner.ID)
	require.NoError(t, err)
	require.Contains(t, ownerDetail.PermissionSources, dto.ProjectPermissionSource{
		Source: models.ProjectAccessSourceOwner,
		Role:   models.ProjectRoleOwner,
	})
	require.Contains(t, ownerDetail.PermissionSources, dto.ProjectPermissionSource{
		Source: models.ProjectAccessSourceWorkspace,
		Role:   models.ProjectRoleEditor,
	})
	require.Contains(t, ownerDetail.PermissionSources, dto.ProjectPermissionSource{
		Source: models.ProjectAccessSourceDirectShare,
		Role:   models.ProjectRoleEditor,
	})
	require.Contains(t, ownerDetail.PermissionSources, dto.ProjectPermissionSource{
		Source: "share_link",
		Role:   models.ProjectRoleViewer,
	})
}

func hashProjectShareTokenForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
