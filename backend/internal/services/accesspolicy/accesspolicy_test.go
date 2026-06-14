package accesspolicy

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestProjectForPublishWithDBPreservesPublishPolicy(t *testing.T) {
	db := testsupport.SetupTestDB()
	owner := models.User{Username: "owner", Email: "owner@example.com"}
	admin := models.User{Username: "admin", Email: "admin@example.com"}
	member := models.User{Username: "member", Email: "member@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&editor).Error)

	workspace := models.Workspace{
		ID:          uuid.New(),
		OwnerUserID: owner.ID,
		Name:        "Team",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      admin.ID,
		Role:        models.WorkspaceRoleAdmin,
	}).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      member.ID,
		Role:        models.WorkspaceRoleMember,
	}).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      editor.ID,
		Role:        models.WorkspaceRoleAdmin,
	}).Error)

	project := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Publish policy",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)

	_, err := ProjectForPublishWithDB(db, project.ID, owner.ID)
	require.NoError(t, err)
	_, err = ProjectForPublishWithDB(db, project.ID, admin.ID)
	require.NoError(t, err)
	_, err = ProjectForPublishWithDB(db, project.ID, member.ID)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = ProjectForPublishWithDB(db, project.ID, editor.ID)
	require.ErrorIs(t, err, ErrForbidden)
}

func TestRequireWorkspaceAccountConnectWithDBUsesWorkspacePermission(t *testing.T) {
	db := testsupport.SetupTestDB()
	owner := models.User{Username: "owner", Email: "owner@example.com"}
	admin := models.User{Username: "admin", Email: "admin@example.com"}
	member := models.User{Username: "member", Email: "member@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&member).Error)

	workspace := models.Workspace{
		ID:          uuid.New(),
		OwnerUserID: owner.ID,
		Name:        "Team",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      admin.ID,
		Role:        models.WorkspaceRoleAdmin,
	}).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      member.ID,
		Role:        models.WorkspaceRoleMember,
	}).Error)

	require.NoError(t, RequireWorkspaceAccountConnectWithDB(db, workspace.ID, owner.ID))
	require.NoError(t, RequireWorkspaceAccountConnectWithDB(db, workspace.ID, admin.ID))
	require.ErrorIs(t, RequireWorkspaceAccountConnectWithDB(db, workspace.ID, member.ID), ErrForbidden)
	require.NoError(t, RequireWorkspaceAccountConnectWithDB(db, models.PersonalWorkspaceID(member.ID), member.ID))
}
