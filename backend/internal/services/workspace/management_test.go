package workspace_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestWorkspaceManagement(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "workspace-owner", Email: "workspace-owner@example.com"}
	admin := models.User{Username: "workspace-admin", Email: "workspace-admin@example.com"}
	member := models.User{Username: "workspace-member", Email: "workspace-member@example.com"}
	viewer := models.User{Username: "workspace-viewer", Email: "workspace-viewer@example.com"}
	stranger := models.User{Username: "workspace-stranger", Email: "workspace-stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&viewer).Error)
	require.NoError(t, db.Create(&stranger).Error)

	workspace, err := s.CreateWorkspace(owner.ID, dto.CreateWorkspaceRequest{
		Name: " Team Workspace ",
		Slug: " team-workspace ",
	})
	require.NoError(t, err)
	require.Equal(t, owner.ID, workspace.OwnerUserID)
	require.Equal(t, "Team Workspace", workspace.Name)
	require.Equal(t, "team-workspace", workspace.Slug)
	require.Equal(t, models.WorkspaceStatusActive, workspace.Status)
	require.Equal(t, models.WorkspaceRoleOwner, workspace.Role)

	var ownerMembership models.WorkspaceMember
	require.NoError(t, db.First(&ownerMembership, "workspace_id = ? AND user_id = ?", workspace.ID, owner.ID).Error)
	require.Equal(t, models.WorkspaceRoleOwner, ownerMembership.Role)
	require.NotNil(t, ownerMembership.JoinedAt)

	addedAdmin, err := s.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		Email: "WORKSPACE-ADMIN@example.com",
		Role:  models.WorkspaceRoleAdmin,
	})
	require.NoError(t, err)
	require.Equal(t, admin.ID, addedAdmin.UserID)
	require.Equal(t, admin.Email, addedAdmin.Email)
	require.Equal(t, models.WorkspaceRoleAdmin, addedAdmin.Role)
	require.NotNil(t, addedAdmin.InvitedBy)
	require.Equal(t, owner.ID, *addedAdmin.InvitedBy)
	require.NotNil(t, addedAdmin.JoinedAt)

	addedMember, err := s.AddWorkspaceMember(workspace.ID, admin.ID, dto.AddWorkspaceMemberRequest{
		UserID: member.ID,
		Role:   models.WorkspaceRoleMember,
	})
	require.NoError(t, err)
	require.Equal(t, member.ID, addedMember.UserID)
	require.Equal(t, models.WorkspaceRoleMember, addedMember.Role)

	addedViewer, err := s.AddWorkspaceMember(workspace.ID, admin.ID, dto.AddWorkspaceMemberRequest{
		UserID: viewer.ID,
		Role:   models.WorkspaceRoleViewer,
	})
	require.NoError(t, err)
	require.Equal(t, viewer.ID, addedViewer.UserID)
	require.Equal(t, models.WorkspaceRoleViewer, addedViewer.Role)

	list, err := s.ListWorkspaceMembers(workspace.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, list.Items, 4)
	require.Equal(t, owner.ID, list.Items[0].UserID)
	require.Equal(t, models.WorkspaceRoleOwner, list.Items[0].Role)

	updatedWorkspace, err := s.UpdateWorkspace(workspace.ID, admin.ID, dto.UpdateWorkspaceRequest{
		Name: "Renamed Workspace",
		Slug: "renamed-workspace",
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed Workspace", updatedWorkspace.Name)
	require.Equal(t, "renamed-workspace", updatedWorkspace.Slug)
	require.Equal(t, models.WorkspaceRoleAdmin, updatedWorkspace.Role)

	updatedMember, err := s.UpdateWorkspaceMember(workspace.ID, admin.ID, member.ID, dto.UpdateWorkspaceMemberRequest{
		Role: models.WorkspaceRoleViewer,
	})
	require.NoError(t, err)
	require.Equal(t, models.WorkspaceRoleViewer, updatedMember.Role)

	workspaces, err := s.ListWorkspaces(member.ID)
	require.NoError(t, err)
	require.Len(t, workspaces.Items, 1)
	require.Equal(t, workspace.ID, workspaces.Items[0].ID)
	require.Equal(t, models.WorkspaceRoleViewer, workspaces.Items[0].Role)

	detail, err := s.GetWorkspace(workspace.ID, viewer.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, detail.ID)
	require.Equal(t, models.WorkspaceRoleViewer, detail.Role)

	_, err = s.AddWorkspaceMember(workspace.ID, viewer.ID, dto.AddWorkspaceMemberRequest{
		UserID: stranger.ID,
		Role:   models.WorkspaceRoleMember,
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	_, err = s.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: owner.ID,
		Role:   models.WorkspaceRoleAdmin,
	})
	require.ErrorIs(t, err, services.ErrInvalidWorkspaceMember)

	_, err = s.UpdateWorkspaceMember(workspace.ID, owner.ID, owner.ID, dto.UpdateWorkspaceMemberRequest{
		Role: models.WorkspaceRoleViewer,
	})
	require.ErrorIs(t, err, services.ErrInvalidWorkspaceMember)

	_, err = s.GetWorkspace(workspace.ID, stranger.ID)
	require.ErrorIs(t, err, services.ErrForbidden)

	require.NoError(t, s.RemoveWorkspaceMember(workspace.ID, admin.ID, viewer.ID))
	_, err = s.GetWorkspace(workspace.ID, viewer.ID)
	require.ErrorIs(t, err, services.ErrForbidden)

	err = s.RemoveWorkspaceMember(workspace.ID, owner.ID, owner.ID)
	require.ErrorIs(t, err, services.ErrInvalidWorkspaceMember)
}

func TestWorkspaceActivitiesRecordManagementChanges(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "activity-owner", Email: "activity-owner@example.com"}
	member := models.User{Username: "activity-member", Email: "activity-member@example.com"}
	stranger := models.User{Username: "activity-stranger", Email: "activity-stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&stranger).Error)

	workspace, err := s.CreateWorkspace(owner.ID, dto.CreateWorkspaceRequest{
		Name: "Activity Workspace",
		Slug: "activity-workspace",
	})
	require.NoError(t, err)
	_, err = s.UpdateWorkspace(workspace.ID, owner.ID, dto.UpdateWorkspaceRequest{
		Name: "Renamed Activity",
		Slug: "renamed-activity",
	})
	require.NoError(t, err)
	_, err = s.UpdateWorkspace(workspace.ID, owner.ID, dto.UpdateWorkspaceRequest{
		Name: " Renamed Activity ",
		Slug: " renamed-activity ",
	})
	require.NoError(t, err)
	_, err = s.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: member.ID,
		Role:   models.WorkspaceRoleMember,
	})
	require.NoError(t, err)
	duplicateMember, err := s.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: member.ID,
		Role:   models.WorkspaceRoleMember,
	})
	require.NoError(t, err)
	require.Equal(t, member.ID, duplicateMember.UserID)
	require.Equal(t, models.WorkspaceRoleMember, duplicateMember.Role)
	_, err = s.UpdateWorkspaceMember(workspace.ID, owner.ID, member.ID, dto.UpdateWorkspaceMemberRequest{
		Role: models.WorkspaceRoleViewer,
	})
	require.NoError(t, err)
	require.NoError(t, s.RemoveWorkspaceMember(workspace.ID, owner.ID, member.ID))

	activities, err := s.ListWorkspaceActivities(workspace.ID, owner.ID, 10)
	require.NoError(t, err)
	require.Len(t, activities.Items, 5)

	eventCounts := map[string]int{}
	for _, activity := range activities.Items {
		eventCounts[activity.EventType]++
		require.Equal(t, workspace.ID, activity.WorkspaceID)
		require.Equal(t, owner.ID, activity.ActorUserID)
		require.Equal(t, owner.Username, activity.ActorUsername)
	}
	require.Equal(t, 1, eventCounts[models.WorkspaceActivityWorkspaceCreated])
	require.Equal(t, 1, eventCounts[models.WorkspaceActivityWorkspaceUpdated])
	require.Equal(t, 1, eventCounts[models.WorkspaceActivityMemberAdded])
	require.Equal(t, 1, eventCounts[models.WorkspaceActivityMemberRoleChanged])
	require.Equal(t, 1, eventCounts[models.WorkspaceActivityMemberRemoved])

	var removed dto.WorkspaceActivity
	for _, activity := range activities.Items {
		if activity.EventType == models.WorkspaceActivityMemberRemoved {
			removed = activity
			break
		}
	}
	require.Equal(t, models.WorkspaceActivityMemberRemoved, removed.EventType)
	require.NotNil(t, removed.TargetUserID)
	require.Equal(t, member.ID, *removed.TargetUserID)
	require.Equal(t, member.Username, removed.TargetUsername)
	require.Equal(t, models.WorkspaceRoleViewer, removed.Metadata["previous_role"])

	_, err = s.ListWorkspaceActivities(workspace.ID, stranger.ID, 10)
	require.ErrorIs(t, err, services.ErrForbidden)
}

func TestWorkspaceProjectFlow(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "team-owner", Email: "team-owner@example.com"}
	member := models.User{Username: "team-member", Email: "team-member@example.com"}
	viewer := models.User{Username: "team-viewer", Email: "team-viewer@example.com"}
	stranger := models.User{Username: "team-stranger", Email: "team-stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&viewer).Error)
	require.NoError(t, db.Create(&stranger).Error)

	workspace, err := s.CreateWorkspace(owner.ID, dto.CreateWorkspaceRequest{Name: "Team"})
	require.NoError(t, err)
	_, err = s.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: member.ID,
		Role:   models.WorkspaceRoleMember,
	})
	require.NoError(t, err)
	_, err = s.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: viewer.ID,
		Role:   models.WorkspaceRoleViewer,
	})
	require.NoError(t, err)

	created, err := s.CreateWorkspaceProject(workspace.ID, member.ID, dto.CreateProjectRequest{
		Title:         "Team Post",
		SourceContent: "<p>shared draft</p>",
		Platforms:     []string{"wechat", "x"},
	})
	require.NoError(t, err)
	require.Equal(t, member.ID, created.UserID)
	require.NotNil(t, created.WorkspaceID)
	require.Equal(t, workspace.ID, *created.WorkspaceID)
	require.Equal(t, models.ProjectRoleOwner, created.Role)
	require.Len(t, created.Publications, 2)

	var stored models.Project
	require.NoError(t, db.First(&stored, "id = ?", created.ID).Error)
	require.NotNil(t, stored.WorkspaceID)
	require.Equal(t, workspace.ID, *stored.WorkspaceID)

	ownerProjects, err := s.ListWorkspaceProjects(workspace.ID, owner.ID, 1, 10, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), ownerProjects.Total)
	ownerItems, ok := ownerProjects.Items.([]dto.ProjectListItem)
	require.True(t, ok)
	require.Len(t, ownerItems, 1)
	require.Equal(t, models.ProjectRoleEditor, ownerItems[0].Role)
	require.Equal(t, models.ProjectAccessSourceWorkspace, ownerItems[0].AccessSource)
	require.NotNil(t, ownerItems[0].WorkspaceID)
	require.Equal(t, workspace.ID, *ownerItems[0].WorkspaceID)

	viewerProjects, err := s.ListWorkspaceProjects(workspace.ID, viewer.ID, 1, 10, "", "")
	require.NoError(t, err)
	viewerItems := viewerProjects.Items.([]dto.ProjectListItem)
	require.Equal(t, models.ProjectRoleViewer, viewerItems[0].Role)
	require.Equal(t, models.ProjectAccessSourceWorkspace, viewerItems[0].AccessSource)

	memberDetail, err := s.GetProject(created.ID, &member.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleOwner, memberDetail.Role)
	require.Equal(t, models.ProjectAccessSourceOwner, memberDetail.AccessSource)
	require.Equal(t, "<p>shared draft</p>", memberDetail.SourceContent)

	ownerDetail, err := s.GetProject(created.ID, &owner.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, ownerDetail.Role)
	require.Equal(t, models.ProjectAccessSourceWorkspace, ownerDetail.AccessSource)

	viewerDetail, err := s.GetProject(created.ID, &viewer.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleViewer, viewerDetail.Role)
	require.Equal(t, models.ProjectAccessSourceWorkspace, viewerDetail.AccessSource)

	updated, err := s.UpdateProject(created.ID, owner.ID, dto.UpdateProjectRequest{
		Title:         "Owner Edited",
		SourceContent: "<p>owner edit</p>",
		Platforms:     []string{"wechat"},
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)
	require.Equal(t, "Owner Edited", updated.Title)

	_, err = s.UpdateProject(created.ID, viewer.ID, dto.UpdateProjectRequest{
		Title:         "Viewer Edit",
		SourceContent: "<p>viewer edit</p>",
		Platforms:     []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	publications, err := s.GetProjectPublications(created.ID, &viewer.ID, false)
	require.NoError(t, err)
	require.Len(t, publications.Items, 2)

	accessible, err := s.ListProjects(1, 10, "", "", "", &owner.ID)
	require.NoError(t, err)
	accessibleItems := accessible.Items.([]dto.ProjectListItem)
	require.Len(t, accessibleItems, 1)
	require.Equal(t, models.ProjectRoleEditor, accessibleItems[0].Role)
	require.Equal(t, models.ProjectAccessSourceWorkspace, accessibleItems[0].AccessSource)

	_, err = s.CreateWorkspaceProject(workspace.ID, viewer.ID, dto.CreateProjectRequest{
		Title:         "Viewer Post",
		SourceContent: "<p>viewer</p>",
		Platforms:     []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	_, err = s.ListWorkspaceProjects(workspace.ID, stranger.ID, 1, 10, "", "")
	require.ErrorIs(t, err, services.ErrForbidden)
}
