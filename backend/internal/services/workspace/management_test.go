package workspace_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
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
	require.Len(t, workspaces.Items, 2)
	var teamWorkspace dto.Workspace
	for _, item := range workspaces.Items {
		if item.ID == workspace.ID {
			teamWorkspace = item
		}
	}
	require.Equal(t, workspace.ID, teamWorkspace.ID)
	require.Equal(t, models.WorkspaceRoleViewer, teamWorkspace.Role)

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

func TestWorkspaceInvites(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "invite-owner", Email: "invite-owner@example.com"}
	member := models.User{Username: "invite-member", Email: "invite-member@example.com"}
	viewer := models.User{Username: "invite-viewer", Email: "invite-viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&viewer).Error)

	workspace, err := s.CreateWorkspace(owner.ID, dto.CreateWorkspaceRequest{Name: "Invite Workspace"})
	require.NoError(t, err)

	invite, err := s.CreateWorkspaceInvite(workspace.ID, owner.ID, dto.CreateWorkspaceInviteRequest{
		Email: "INVITE-MEMBER@example.com",
		Role:  models.WorkspaceRoleMember,
	})
	require.NoError(t, err)
	require.NotEmpty(t, invite.Token)
	require.Equal(t, models.WorkspaceInviteStatusPending, invite.Status)

	list, err := s.ListWorkspaceInvites(workspace.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, invite.ID, list.Items[0].ID)

	accepted, err := s.AcceptWorkspaceInvite(member.ID, dto.AcceptWorkspaceInviteRequest{Token: invite.Token})
	require.NoError(t, err)
	require.Equal(t, member.ID, accepted.UserID)
	require.Equal(t, models.WorkspaceRoleMember, accepted.Role)

	_, err = s.AcceptWorkspaceInvite(member.ID, dto.AcceptWorkspaceInviteRequest{Token: invite.Token})
	require.ErrorIs(t, err, services.ErrInvalidWorkspaceInvite)

	_, err = s.CreateWorkspaceInvite(workspace.ID, member.ID, dto.CreateWorkspaceInviteRequest{
		Email: "invite-viewer@example.com",
		Role:  models.WorkspaceRoleViewer,
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	revokedInvite, err := s.CreateWorkspaceInvite(workspace.ID, owner.ID, dto.CreateWorkspaceInviteRequest{
		Email: "invite-viewer@example.com",
		Role:  models.WorkspaceRoleViewer,
	})
	require.NoError(t, err)
	require.NoError(t, s.RevokeWorkspaceInvite(workspace.ID, owner.ID, revokedInvite.ID))
	_, err = s.AcceptWorkspaceInvite(viewer.ID, dto.AcceptWorkspaceInviteRequest{Token: revokedInvite.Token})
	require.ErrorIs(t, err, services.ErrInvalidWorkspaceInvite)
}

func TestGetWorkspaceUsesWriterForScopedAccess(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	ownerID := uuid.New()
	owner := models.User{ID: ownerID, Username: "workspace-route-owner", Email: "workspace-route-owner@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	require.NoError(t, reader.Create(&owner).Error)

	workspaceID := uuid.New()
	writerWorkspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerID,
		Name:        "Writer Workspace",
		Slug:        "writer-workspace",
		Status:      models.WorkspaceStatusActive,
	}
	readerWorkspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerID,
		Name:        "Stale Reader Workspace",
		Slug:        "stale-reader-workspace",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, writer.Create(&writerWorkspace).Error)
	require.NoError(t, reader.Create(&readerWorkspace).Error)

	detail, err := s.GetWorkspace(workspaceID, ownerID)
	require.NoError(t, err)
	require.Equal(t, "Writer Workspace", detail.Name)
	require.Equal(t, "writer-workspace", detail.Slug)
}

func TestListWorkspacesUsesWriterForScopedAccess(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	userID := uuid.New()
	ownerID := uuid.New()
	staleOwnerID := uuid.New()
	user := models.User{ID: userID, Username: "workspace-list-user", Email: "workspace-list-user@example.com"}
	owner := models.User{ID: ownerID, Username: "workspace-list-owner", Email: "workspace-list-owner@example.com"}
	staleOwner := models.User{ID: staleOwnerID, Username: "workspace-list-stale-owner", Email: "workspace-list-stale-owner@example.com"}
	require.NoError(t, writer.Create(&user).Error)
	require.NoError(t, writer.Create(&owner).Error)
	require.NoError(t, reader.Create(&user).Error)
	require.NoError(t, reader.Create(&staleOwner).Error)

	currentWorkspace := models.Workspace{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Name:        "Current Workspace",
		Status:      models.WorkspaceStatusActive,
	}
	staleWorkspace := models.Workspace{
		ID:          uuid.New(),
		OwnerUserID: staleOwnerID,
		Name:        "Stale Reader Workspace",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, writer.Create(&currentWorkspace).Error)
	require.NoError(t, writer.Create(&models.WorkspaceMember{
		WorkspaceID: currentWorkspace.ID,
		UserID:      userID,
		Role:        models.WorkspaceRoleViewer,
	}).Error)
	require.NoError(t, reader.Create(&staleWorkspace).Error)
	require.NoError(t, reader.Create(&models.WorkspaceMember{
		WorkspaceID: staleWorkspace.ID,
		UserID:      userID,
		Role:        models.WorkspaceRoleAdmin,
	}).Error)

	res, err := s.ListWorkspaces(userID)
	require.NoError(t, err)

	workspacesByID := make(map[uuid.UUID]dto.Workspace, len(res.Items))
	for _, item := range res.Items {
		workspacesByID[item.ID] = item
	}
	require.Len(t, workspacesByID, 2)
	require.Contains(t, workspacesByID, models.PersonalWorkspaceID(userID))
	require.Contains(t, workspacesByID, currentWorkspace.ID)
	require.NotContains(t, workspacesByID, staleWorkspace.ID)
	require.Equal(t, models.WorkspaceRoleViewer, workspacesByID[currentWorkspace.ID].Role)
}

func TestListWorkspaceMembersUsesWriterForScopedAccess(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	ownerID := uuid.New()
	currentMemberID := uuid.New()
	staleMemberID := uuid.New()
	owner := models.User{ID: ownerID, Username: "workspace-members-owner", Email: "workspace-members-owner@example.com"}
	currentMember := models.User{ID: currentMemberID, Username: "workspace-members-current", Email: "workspace-members-current@example.com"}
	staleMember := models.User{ID: staleMemberID, Username: "workspace-members-stale", Email: "workspace-members-stale@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	require.NoError(t, writer.Create(&currentMember).Error)
	require.NoError(t, reader.Create(&owner).Error)
	require.NoError(t, reader.Create(&staleMember).Error)

	workspaceID := uuid.New()
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerID,
		Name:        "Members Workspace",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, writer.Create(&workspace).Error)
	require.NoError(t, reader.Create(&workspace).Error)
	require.NoError(t, writer.Create(&models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      ownerID,
		Role:        models.WorkspaceRoleOwner,
	}).Error)
	require.NoError(t, writer.Create(&models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      currentMemberID,
		Role:        models.WorkspaceRoleMember,
	}).Error)
	require.NoError(t, reader.Create(&models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      ownerID,
		Role:        models.WorkspaceRoleOwner,
	}).Error)
	require.NoError(t, reader.Create(&models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      staleMemberID,
		Role:        models.WorkspaceRoleAdmin,
	}).Error)

	res, err := s.ListWorkspaceMembers(workspaceID, ownerID)
	require.NoError(t, err)

	membersByID := make(map[uuid.UUID]dto.WorkspaceMember, len(res.Items))
	for _, item := range res.Items {
		membersByID[item.UserID] = item
	}
	require.Len(t, membersByID, 2)
	require.Contains(t, membersByID, ownerID)
	require.Contains(t, membersByID, currentMemberID)
	require.NotContains(t, membersByID, staleMemberID)
	require.Equal(t, models.WorkspaceRoleMember, membersByID[currentMemberID].Role)
}

func TestListWorkspaceProjectsUsesWriterForScopedAccess(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	ownerID := uuid.New()
	owner := models.User{ID: ownerID, Username: "workspace-projects-owner", Email: "workspace-projects-owner@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	require.NoError(t, reader.Create(&owner).Error)

	workspaceID := uuid.New()
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerID,
		Name:        "Projects Workspace",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, writer.Create(&workspace).Error)
	require.NoError(t, reader.Create(&workspace).Error)

	writerProject := models.Project{
		ID:            uuid.New(),
		UserID:        ownerID,
		WorkspaceID:   &workspaceID,
		Title:         "Writer Project",
		SourceContent: "writer content",
		Status:        models.ProjectStatusReady,
	}
	staleReaderProject := models.Project{
		ID:            uuid.New(),
		UserID:        ownerID,
		WorkspaceID:   &workspaceID,
		Title:         "Stale Reader Project",
		SourceContent: "reader content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, writer.Create(&writerProject).Error)
	require.NoError(t, reader.Create(&staleReaderProject).Error)

	res, err := s.ListWorkspaceProjects(workspaceID, ownerID, 1, 10, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	items := res.Items.([]dto.ProjectListItem)
	require.Len(t, items, 1)
	require.Equal(t, writerProject.ID, items[0].ID)
	require.Equal(t, "Writer Project", items[0].Title)
}

func TestListWorkspaceInvitesUsesWriterForScopedAccess(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	ownerID := uuid.New()
	owner := models.User{ID: ownerID, Username: "workspace-invites-owner", Email: "workspace-invites-owner@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	require.NoError(t, reader.Create(&owner).Error)

	workspaceID := uuid.New()
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerID,
		Name:        "Invites Workspace",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, writer.Create(&workspace).Error)
	require.NoError(t, reader.Create(&workspace).Error)

	writerInvite := models.WorkspaceInvite{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Email:       "current-invite@example.com",
		Role:        models.WorkspaceRoleMember,
		InvitedBy:   ownerID,
		Status:      models.WorkspaceInviteStatusPending,
		TokenHash:   "writer-invite-token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}
	staleReaderInvite := models.WorkspaceInvite{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Email:       "stale-invite@example.com",
		Role:        models.WorkspaceRoleAdmin,
		InvitedBy:   ownerID,
		Status:      models.WorkspaceInviteStatusPending,
		TokenHash:   "reader-invite-token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, writer.Create(&writerInvite).Error)
	require.NoError(t, reader.Create(&staleReaderInvite).Error)

	res, err := s.ListWorkspaceInvites(workspaceID, ownerID)
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, writerInvite.ID, res.Items[0].ID)
	require.Equal(t, "current-invite@example.com", res.Items[0].Email)
}

func TestListWorkspaceActivitiesUsesWriterForScopedAccess(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	ownerID := uuid.New()
	owner := models.User{ID: ownerID, Username: "workspace-activities-owner", Email: "workspace-activities-owner@example.com"}
	require.NoError(t, writer.Create(&owner).Error)
	require.NoError(t, reader.Create(&owner).Error)

	workspaceID := uuid.New()
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerID,
		Name:        "Activities Workspace",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, writer.Create(&workspace).Error)
	require.NoError(t, reader.Create(&workspace).Error)

	writerActivity := models.WorkspaceActivity{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		ActorUserID: ownerID,
		EventType:   models.WorkspaceActivityWorkspaceUpdated,
		Metadata:    datatypes.JSON([]byte(`{"source":"writer"}`)),
	}
	staleReaderActivity := models.WorkspaceActivity{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		ActorUserID: ownerID,
		EventType:   models.WorkspaceActivityMemberRemoved,
		Metadata:    datatypes.JSON([]byte(`{"source":"reader"}`)),
	}
	require.NoError(t, writer.Create(&writerActivity).Error)
	require.NoError(t, reader.Create(&staleReaderActivity).Error)

	res, err := s.ListWorkspaceActivities(workspaceID, ownerID, 10)
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, writerActivity.ID, res.Items[0].ID)
	require.Equal(t, models.WorkspaceActivityWorkspaceUpdated, res.Items[0].EventType)
	require.Equal(t, "writer", res.Items[0].Metadata["source"])
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
