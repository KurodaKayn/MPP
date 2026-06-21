package project_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestDeleteProjectRemovesOwnerProjectAndDependents(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(
		&models.MediaAsset{},
		&models.PublishEvent{},
		&models.ScheduledPublication{},
		&models.PublishAttempt{},
	))
	s := services.NewDashboardService(db)

	owner := models.User{
		Username:     "delete-owner",
		Email:        "delete-owner@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&owner).Error)
	workspaceID := models.PersonalWorkspaceID(owner.ID)
	require.NoError(t, db.Create(&models.Workspace{
		ID:          workspaceID,
		OwnerUserID: owner.ID,
		Name:        models.PersonalWorkspaceName,
		Slug:        models.PersonalWorkspaceSlug(owner.ID),
		Status:      models.WorkspaceStatusActive,
	}).Error)

	project := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspaceID,
		Title:         "Delete me",
		SourceContent: "<p>Body</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusDraft,
	}
	require.NoError(t, db.Create(&publication).Error)
	schedule := models.ScheduledPublication{
		WorkspaceID:    workspaceID,
		ProjectID:      project.ID,
		PublicationID:  publication.ID,
		ScheduledAt:    time.Now().Add(time.Hour),
		Timezone:       "UTC",
		Status:         models.ScheduledPublicationStatusScheduled,
		IdempotencyKey: "delete-test",
		CreatedBy:      owner.ID,
	}
	require.NoError(t, db.Create(&schedule).Error)
	require.NoError(t, db.Create(&models.PublishAttempt{
		ScheduledPublicationID: schedule.ID,
		AttemptNo:              1,
		StartedAt:              time.Now(),
		Status:                 models.PublishAttemptStatusFailed,
	}).Error)
	require.NoError(t, db.Create(&models.PublishEvent{
		PublicationID:  publication.ID,
		ProjectID:      project.ID,
		UserID:         owner.ID,
		Platform:       "wechat",
		JobID:          uuid.New(),
		IdempotencyKey: "delete-test",
		EventType:      "queued",
		Status:         "queued",
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    owner.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectActivity{
		ProjectID:   project.ID,
		ActorUserID: owner.ID,
		EventType:   models.ProjectActivityContentSaved,
		Metadata:    datatypes.JSON([]byte(`{}`)),
	}).Error)
	require.NoError(t, db.Create(&models.ProjectComment{
		ProjectID: project.ID,
		AuthorID:  owner.ID,
		Body:      "Comment",
		Status:    models.ProjectCommentStatusOpen,
		Metadata:  datatypes.JSON([]byte(`{}`)),
	}).Error)
	require.NoError(t, db.Create(&models.ProjectVersion{
		ProjectID:     project.ID,
		CreatedBy:     owner.ID,
		VersionNumber: 1,
		Title:         project.Title,
		SourceContent: project.SourceContent,
		Source:        "test",
	}).Error)
	require.NoError(t, db.Create(&models.ProjectShareLink{
		ProjectID: project.ID,
		CreatedBy: owner.ID,
		TokenHash: "delete-test-token",
		Role:      models.ProjectRoleViewer,
		Status:    models.ProjectShareLinkStatusActive,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectListSummary{
		ProjectID:    project.ID,
		UserID:       owner.ID,
		WorkspaceID:  workspaceID,
		Title:        project.Title,
		Status:       project.Status,
		Publications: datatypes.JSON([]byte(`[]`)),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		RefreshedAt:  time.Now(),
	}).Error)
	assetID := uuid.New()
	require.NoError(t, db.Create(&models.MediaAsset{
		ID:               assetID,
		UserID:           owner.ID,
		WorkspaceID:      &workspaceID,
		ProjectID:        &project.ID,
		Bucket:           "mpp-media",
		ObjectKey:        "projects/delete-me/image.png",
		OriginalFilename: "image.png",
		MimeType:         "image/png",
		SizeBytes:        42,
		Usage:            models.MediaAssetUsageEditorImage,
		LibraryScope:     models.MediaAssetLibraryScopeProject,
		Status:           models.MediaAssetStatusReady,
	}).Error)
	require.NoError(t, db.Create(&models.MediaAssetUsage{
		MediaAssetID: assetID,
		WorkspaceID:  workspaceID,
		ProjectID:    &project.ID,
		ResourceType: "project",
		ResourceID:   project.ID,
		UsageKind:    models.MediaAssetUsageEditorImage,
	}).Error)
	require.NoError(t, db.Create(&models.PlatformAccountGrant{
		PlatformAccountID: uuid.New(),
		WorkspaceID:       workspaceID,
		ProjectID:         &project.ID,
		Role:              models.PlatformAccountGrantRolePublisher,
		CreatedBy:         owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ExtensionCallbackToken{
		ExecutionID: "execution-1",
		ProjectID:   project.ID,
		UserID:      owner.ID,
		Platform:    "wechat",
		Token:       "callback-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}).Error)
	extensionEvent := models.ExtensionExecutionEvent{
		CallbackTokenID: uuid.New(),
		ExecutionID:     "execution-1",
		ProjectID:       project.ID,
		UserID:          owner.ID,
		EventID:         "event-1",
		Platform:        "wechat",
		Status:          "queued",
		Metadata:        datatypes.JSON([]byte(`{}`)),
	}
	require.NoError(t, db.Create(&extensionEvent).Error)
	require.NoError(t, db.Create(&models.ExtensionExecutionEventClaim{
		EventID:  extensionEvent.EventID,
		RecordID: extensionEvent.ID,
	}).Error)

	err := s.DeleteProject(project.ID, owner.ID)

	require.NoError(t, err)
	require.Zero(t, countRows(t, db, &models.Project{}, "id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectPlatformPublication{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectCollaborator{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectActivity{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectComment{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectVersion{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectShareLink{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ProjectListSummary{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.MediaAssetUsage{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.PlatformAccountGrant{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ExtensionCallbackToken{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ExtensionExecutionEvent{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ExtensionExecutionEventClaim{}, "record_id = ?", extensionEvent.ID))
	require.Zero(t, countRows(t, db, &models.PublishEvent{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.ScheduledPublication{}, "project_id = ?", project.ID))
	require.Zero(t, countRows(t, db, &models.PublishAttempt{}, "scheduled_publication_id = ?", schedule.ID))

	var asset models.MediaAsset
	require.NoError(t, db.Unscoped().First(&asset, "id = ?", assetID).Error)
	require.Nil(t, asset.ProjectID)
}

func TestDeleteProjectAllowsWorkspaceAdminAndRejectsNonOwners(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "workspace-owner", Email: "workspace-owner@example.com", PasswordHash: "hash"}
	admin := models.User{Username: "workspace-admin", Email: "workspace-admin@example.com", PasswordHash: "hash"}
	member := models.User{Username: "workspace-member", Email: "workspace-member@example.com", PasswordHash: "hash"}
	collaborator := models.User{Username: "direct-collaborator", Email: "direct-collaborator@example.com", PasswordHash: "hash"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&collaborator).Error)

	workspace := models.Workspace{
		OwnerUserID: owner.ID,
		Name:        "Team",
		Slug:        "team",
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

	adminDeletable := models.Project{
		UserID:        member.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Admin deletable",
		SourceContent: "body",
		Status:        models.ProjectStatusReady,
	}
	memberBlocked := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Member blocked",
		SourceContent: "body",
		Status:        models.ProjectStatusReady,
	}
	collaboratorBlocked := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Collaborator blocked",
		SourceContent: "body",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&adminDeletable).Error)
	require.NoError(t, db.Create(&memberBlocked).Error)
	require.NoError(t, db.Create(&collaboratorBlocked).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: adminDeletable.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusDraft,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: memberBlocked.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusDraft,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: collaboratorBlocked.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusDraft,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: collaboratorBlocked.ID,
		UserID:    collaborator.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)

	require.NoError(t, s.DeleteProject(adminDeletable.ID, admin.ID))
	require.Zero(t, countRows(t, db, &models.Project{}, "id = ?", adminDeletable.ID))

	require.ErrorIs(t, s.DeleteProject(memberBlocked.ID, member.ID), services.ErrForbidden)
	require.Equal(t, int64(1), countRows(t, db, &models.Project{}, "id = ?", memberBlocked.ID))

	require.ErrorIs(t, s.DeleteProject(collaboratorBlocked.ID, collaborator.ID), services.ErrForbidden)
	require.Equal(t, int64(1), countRows(t, db, &models.Project{}, "id = ?", collaboratorBlocked.ID))
}

func TestDeleteProjectBlocksActivePublishingWork(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.ScheduledPublication{}))
	s := services.NewDashboardService(db)

	owner := models.User{Username: "active-owner", Email: "active-owner@example.com", PasswordHash: "hash"}
	require.NoError(t, db.Create(&owner).Error)
	workspaceID := models.PersonalWorkspaceID(owner.ID)

	queuedProject := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspaceID,
		Title:         "Queued publish",
		SourceContent: "body",
		Status:        models.ProjectStatusPublishing,
	}
	require.NoError(t, db.Create(&queuedProject).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: queuedProject.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusQueued,
	}).Error)

	err := s.DeleteProject(queuedProject.ID, owner.ID)

	require.ErrorIs(t, err, services.ErrProjectDeletionBlocked)
	require.Equal(t, int64(1), countRows(t, db, &models.Project{}, "id = ?", queuedProject.ID))

	scheduledProject := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspaceID,
		Title:         "Manual action",
		SourceContent: "body",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&scheduledProject).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID: scheduledProject.ID,
		Platform:  "zhihu",
		Enabled:   true,
		Status:    models.PublicationStatusDraft,
	}
	require.NoError(t, db.Create(&publication).Error)
	require.NoError(t, db.Create(&models.ScheduledPublication{
		WorkspaceID:   workspaceID,
		ProjectID:     scheduledProject.ID,
		PublicationID: publication.ID,
		ScheduledAt:   time.Now().Add(-time.Hour),
		Timezone:      "UTC",
		Status:        models.ScheduledPublicationStatusNeedsManualAction,
		CreatedBy:     owner.ID,
	}).Error)

	err = s.DeleteProject(scheduledProject.ID, owner.ID)

	require.ErrorIs(t, err, services.ErrProjectDeletionBlocked)
	require.Equal(t, int64(1), countRows(t, db, &models.Project{}, "id = ?", scheduledProject.ID))
}

func countRows(t *testing.T, db *gorm.DB, model any, query string, args ...any) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(model).Where(query, args...).Count(&count).Error)
	return count
}
