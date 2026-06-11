package project_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestListProjects(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test"}
	u2 := models.User{Username: "other"}
	db.Create(&u1)
	db.Create(&u2)

	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished, CreatedAt: time.Now().Add(-1 * time.Hour)}
	p2 := models.Project{UserID: u1.ID, Title: "p2", SourceContent: "c2", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	p3 := models.Project{UserID: u2.ID, Title: "p3", SourceContent: "c3", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	db.Create(&p1)
	db.Create(&p2)
	db.Create(&p3)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished, PublishURL: "url1"})

	// Test global admin pagination
	res, err := s.ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.Total)

	// Test Personal scope (u1)
	resScoped, errScoped := s.ListProjects(1, 10, "", "", "", &u1.ID)
	require.NoError(t, errScoped)
	assert.Equal(t, int64(2), resScoped.Total)
	items := resScoped.Items.([]dto.ProjectListItem)
	assert.Len(t, items, 2)
	// Ensure p3 is not in list
	for _, item := range items {
		assert.NotEqual(t, p3.ID, item.ID)
	}
}

func TestListProjectsIncludesCollaboratorProjectsWithRoles(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	collaborator := models.User{Username: "collaborator", Email: "collab@example.com"}
	hiddenOwner := models.User{Username: "hidden-owner", Email: "hidden@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&collaborator).Error)
	require.NoError(t, db.Create(&hiddenOwner).Error)

	ownedProject := models.Project{UserID: collaborator.ID, Title: "Owned", SourceContent: "owned", Status: models.ProjectStatusDraft, CreatedAt: time.Now().Add(2 * time.Hour)}
	sharedProject := models.Project{UserID: owner.ID, Title: "Shared", SourceContent: "shared", Status: models.ProjectStatusReady, CreatedAt: time.Now().Add(time.Hour)}
	hiddenProject := models.Project{UserID: hiddenOwner.ID, Title: "Hidden", SourceContent: "hidden", Status: models.ProjectStatusReady, CreatedAt: time.Now().Add(3 * time.Hour)}
	require.NoError(t, db.Create(&ownedProject).Error)
	require.NoError(t, db.Create(&sharedProject).Error)
	require.NoError(t, db.Create(&hiddenProject).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: sharedProject.ID,
		UserID:    collaborator.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	res, err := s.ListProjects(1, 10, "", "", "", &collaborator.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), res.Total)

	items := res.Items.([]dto.ProjectListItem)
	require.Len(t, items, 2)
	roles := map[uuid.UUID]string{}
	accessSources := map[uuid.UUID]string{}
	for _, item := range items {
		roles[item.ID] = item.Role
		accessSources[item.ID] = item.AccessSource
		require.NotEqual(t, hiddenProject.ID, item.ID)
	}
	require.Equal(t, models.ProjectRoleOwner, roles[ownedProject.ID])
	require.Equal(t, models.ProjectAccessSourceOwner, accessSources[ownedProject.ID])
	require.Equal(t, models.ProjectRoleViewer, roles[sharedProject.ID])
	require.Equal(t, models.ProjectAccessSourceDirectShare, accessSources[sharedProject.ID])
}

func TestListProjectsUsesReaderForAdminList(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	user := models.User{Username: "reader-owner", Email: "reader-owner@example.com"}
	require.NoError(t, reader.Create(&user).Error)
	project := models.Project{UserID: user.ID, Title: "Reader project", SourceContent: "content", Status: models.ProjectStatusReady, CreatedAt: time.Now()}
	require.NoError(t, reader.Create(&project).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	var writerProjects int64
	require.NoError(t, writer.Model(&models.Project{}).Count(&writerProjects).Error)
	require.Equal(t, int64(0), writerProjects)

	res, err := s.ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	items := res.Items.([]dto.ProjectListItem)
	require.Len(t, items, 1)
	require.Equal(t, project.ID, items[0].ID)
}

func TestListProjectsUsesWriterForStickyAdminList(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))
	s := services.NewDashboardServiceWithRouter(writer, router).WithContext(stickyCtx)

	writerUser := models.User{Username: "writer-owner", Email: "writer-owner@example.com", PasswordHash: "hash"}
	require.NoError(t, writer.Create(&writerUser).Error)
	writerProject := models.Project{
		UserID:        writerUser.ID,
		Title:         "Writer project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
		CreatedAt:     time.Now(),
	}
	require.NoError(t, writer.Create(&writerProject).Error)

	readerUser := models.User{Username: "stale-reader-owner", Email: "stale-reader-owner@example.com", PasswordHash: "hash"}
	require.NoError(t, reader.Create(&readerUser).Error)
	staleReaderProject := models.Project{
		UserID:        readerUser.ID,
		Title:         "Stale reader project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
		CreatedAt:     time.Now().Add(time.Hour),
	}
	require.NoError(t, reader.Create(&staleReaderProject).Error)

	res, err := s.ListProjects(1, 10, "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	items := res.Items.([]dto.ProjectListItem)
	require.Len(t, items, 1)
	require.Equal(t, writerProject.ID, items[0].ID)
}

func TestListProjectsUsesWriterForScopedList(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	user := models.User{Username: "scoped-owner", Email: "scoped-owner@example.com"}
	require.NoError(t, writer.Create(&user).Error)
	currentProject := models.Project{UserID: user.ID, Title: "Current project", SourceContent: "content", Status: models.ProjectStatusReady, CreatedAt: time.Now()}
	require.NoError(t, writer.Create(&currentProject).Error)

	staleProject := models.Project{UserID: user.ID, Title: "Stale reader project", SourceContent: "content", Status: models.ProjectStatusReady, CreatedAt: time.Now().Add(time.Hour)}
	require.NoError(t, reader.Create(&staleProject).Error)

	res, err := s.ListProjects(1, 10, "", "", "", &user.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	items := res.Items.([]dto.ProjectListItem)
	require.Len(t, items, 1)
	require.Equal(t, currentProject.ID, items[0].ID)
}

func TestGetProjectUsesReaderForAdminDetail(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	user := models.User{Username: "reader-detail-owner", Email: "reader-detail-owner@example.com"}
	require.NoError(t, reader.Create(&user).Error)
	project := models.Project{UserID: user.ID, Title: "Reader detail", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&project).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID:  project.ID,
		Platform:   "wechat",
		Status:     models.PublicationStatusPublished,
		PublishURL: "https://example.test/reader",
	}).Error)

	detail, err := s.GetProject(project.ID, nil)
	require.NoError(t, err)
	require.Equal(t, project.ID, detail.ID)
	require.Equal(t, "Reader detail", detail.Title)
	require.Len(t, detail.Publications, 1)
	require.Equal(t, "wechat", detail.Publications[0].Platform)
}

func TestGetProjectUsesWriterForStickyAdminDetail(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))
	s := services.NewDashboardServiceWithRouter(writer, router).WithContext(stickyCtx)

	writerUser := models.User{Username: "writer-detail-owner", Email: "writer-detail-owner@example.com"}
	require.NoError(t, writer.Create(&writerUser).Error)
	writerProject := models.Project{UserID: writerUser.ID, Title: "Writer detail", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, writer.Create(&writerProject).Error)

	readerUser := models.User{Username: "reader-detail-owner", Email: "reader-detail-owner@example.com"}
	require.NoError(t, reader.Create(&readerUser).Error)
	staleReaderProject := models.Project{UserID: readerUser.ID, Title: "Stale reader detail", SourceContent: "content", Status: models.ProjectStatusReady}
	require.NoError(t, reader.Create(&staleReaderProject).Error)

	detail, err := s.GetProject(writerProject.ID, nil)
	require.NoError(t, err)
	require.Equal(t, writerProject.ID, detail.ID)
	require.Equal(t, "Writer detail", detail.Title)
}

func TestGetProjectBackfillsPersonalWorkspaceIDWhenLegacyProjectOmitsIt(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "legacy-owner", Email: "legacy-owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Legacy personal project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Model(&models.Project{}).
		Where("id = ?", project.ID).
		Update("workspace_id", nil).Error)

	detail, err := s.GetProject(project.ID, &user.ID)

	require.NoError(t, err)
	require.NotNil(t, detail.WorkspaceID)
	require.Equal(t, models.PersonalWorkspaceID(user.ID), *detail.WorkspaceID)
}

func TestCreateProjectCreatesSelectedPublications(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	resp, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "WeChat title",
		SourceContent: "<p>Hello WeChat</p>",
		Summary:       "Hello WeChat",
		CoverImageURL: "data:image/png;base64,aGVsbG8=",
		Platforms:     []string{"wechat", "wechat", "douyin"},
	})

	require.NoError(t, err)
	assert.Equal(t, "WeChat title", resp.Title)
	assert.Equal(t, models.ProjectStatusReady, resp.Status)
	assert.Equal(t, models.ProjectRoleOwner, resp.Role)
	assert.Len(t, resp.Publications, 2)

	var project models.Project
	require.NoError(t, db.First(&project, "id = ?", resp.ID).Error)
	assert.Equal(t, user.ID, project.UserID)
	assert.Equal(t, "<p>Hello WeChat</p>", project.SourceContent)
	assert.NotNil(t, project.WorkspaceID)
	assert.Equal(t, models.PersonalWorkspaceID(user.ID), *project.WorkspaceID)

	assert.NotNil(t, resp.WorkspaceID)
	assert.Equal(t, models.PersonalWorkspaceID(user.ID), *resp.WorkspaceID)

	var personalWorkspace models.Workspace
	require.NoError(t, db.First(&personalWorkspace, "id = ?", models.PersonalWorkspaceID(user.ID)).Error)
	assert.Equal(t, user.ID, personalWorkspace.OwnerUserID)
	assert.Equal(t, models.PersonalWorkspaceName, personalWorkspace.Name)

	var ownerMembership models.WorkspaceMember
	require.NoError(t, db.First(&ownerMembership, "workspace_id = ? AND user_id = ?", models.PersonalWorkspaceID(user.ID), user.ID).Error)
	assert.Equal(t, models.WorkspaceRoleOwner, ownerMembership.Role)

	workspaceProjects, err := s.ListWorkspaceProjects(models.PersonalWorkspaceID(user.ID), user.ID, 1, 10, "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), workspaceProjects.Total)

	var wechatPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", resp.ID, "wechat").Error)
	assert.Equal(t, models.PublicationStatusPending, wechatPub.Status)

	var config map[string]string
	require.NoError(t, json.Unmarshal(wechatPub.Config, &config))
	assert.Equal(t, "WeChat title", config["title"])
	assert.Equal(t, "Hello WeChat", config["digest"])
	assert.Equal(t, "data:image/png;base64,aGVsbG8=", config["cover_image_url"])

	var adapted map[string]string
	require.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &adapted))
	assert.Empty(t, adapted)

	var douyinPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&douyinPub, "project_id = ? AND platform = ?", resp.ID, "douyin").Error)
	assert.Equal(t, models.PublicationStatusPending, douyinPub.Status)
}

func TestCreateProjectAppliesContentTemplateDefaults(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	ownerID := user.ID
	template := models.ContentTemplate{
		Scope:            models.ContentTemplateScopePersonal,
		OwnerUserID:      &ownerID,
		Name:             "Launch article",
		TitleTemplate:    "Launch {{product}}",
		SourceTemplate:   "<h2>Intro</h2><p>Default body</p>",
		DefaultPlatforms: datatypes.JSON(`["wechat","zhihu"]`),
		PlatformConfig:   datatypes.JSON(`{"wechat":{"digest":"Template digest"}}`),
		Tags:             datatypes.JSON(`["launch"]`),
	}
	require.NoError(t, db.Create(&template).Error)

	resp, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		TemplateID: &template.ID,
	})

	require.NoError(t, err)
	require.Equal(t, template.ID, *resp.TemplateID)
	require.Equal(t, "Launch {{product}}", resp.Title)
	require.Len(t, resp.Publications, 2)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", resp.ID).Error)
	require.Equal(t, "<h2>Intro</h2><p>Default body</p>", saved.SourceContent)

	var wechatPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", resp.ID, "wechat").Error)
	require.True(t, wechatPub.SyncRequired)
	require.Equal(t, models.PublicationDraftStatusUnsynced, wechatPub.DraftStatus)

	var config map[string]string
	require.NoError(t, json.Unmarshal(wechatPub.Config, &config))
	require.Equal(t, "Template digest", config["digest"])
	require.Equal(t, "Launch {{product}}", config["title"])
}

func TestCreateContentTemplateRecordsMediaAssetUsages(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.MediaAsset{}))
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	workspaceID := models.PersonalWorkspaceID(user.ID)
	require.NoError(t, db.Create(&models.Workspace{
		ID:          workspaceID,
		OwnerUserID: user.ID,
		Name:        models.PersonalWorkspaceName,
		Slug:        "personal-" + user.ID.String(),
		Status:      models.WorkspaceStatusActive,
	}).Error)
	assetID := uuid.New()
	require.NoError(t, db.Create(&models.MediaAsset{
		ID:               assetID,
		UserID:           user.ID,
		WorkspaceID:      &workspaceID,
		Bucket:           "mpp-media",
		ObjectKey:        "workspaces/" + workspaceID.String() + "/library/" + assetID.String() + "/hero.png",
		OriginalFilename: "hero.png",
		MimeType:         "image/png",
		SizeBytes:        12,
		Usage:            models.MediaAssetUsageEditorImage,
		LibraryScope:     models.MediaAssetLibraryScopeWorkspace,
		Status:           models.MediaAssetStatusReady,
	}).Error)
	mediaRef := "mpp://media/" + assetID.String()

	template, err := s.CreateContentTemplate(user.ID, workspaceID, dto.CreateContentTemplateRequest{
		Name:             "Media template",
		TitleTemplate:    "Media title",
		SourceTemplate:   "<p>" + mediaRef + "</p>",
		DefaultPlatforms: []string{"wechat"},
		PlatformConfig: map[string]any{
			"wechat": map[string]any{
				"cover_image_url": mediaRef,
			},
		},
	})

	require.NoError(t, err)
	var usages []models.MediaAssetUsage
	require.NoError(t, db.Find(&usages, "template_id = ?", template.ID).Error)
	require.Len(t, usages, 1)
	require.Equal(t, assetID, usages[0].MediaAssetID)
	require.Equal(t, workspaceID, usages[0].WorkspaceID)
	require.Equal(t, "template", usages[0].ResourceType)
	require.Equal(t, template.ID, usages[0].ResourceID)
	require.NotNil(t, usages[0].TemplateID)
	require.Equal(t, template.ID, *usages[0].TemplateID)
}

func TestCreateProjectSanitizesStoredSourceContent(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	resp, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title: "Sanitized title",
		SourceContent: `<p onclick="alert(1)">Hello</p>
			<img src="javascript:alert(1)" onerror="alert(1)" alt="cover">
			<a href="java&#x0A;script:alert(1)">bad link</a>
			<svg onload="alert(1)"></svg>
			<script>alert(1)</script>`,
		Platforms: []string{"wechat"},
	})

	require.NoError(t, err)
	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", resp.ID).Error)
	require.Contains(t, saved.SourceContent, "<p>Hello</p>")
	require.Contains(t, saved.SourceContent, `<img alt="cover"/>`)
	assertStoredHTMLHasNoActiveContent(t, saved.SourceContent)
}

func TestCreateProjectRejectsInvalidInput(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "Missing platform",
		SourceContent: "content",
	})
	require.ErrorIs(t, err, services.ErrInvalidProject)

	_, err = s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "Unknown platform",
		SourceContent: "content",
		Platforms:     []string{"threads"},
	})
	require.ErrorIs(t, err, services.ErrInvalidProject)
}

func TestGetProjectReturnsSourceContentForOwner(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	db.Create(&owner)
	db.Create(&editor)
	db.Create(&stranger)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Existing post",
		SourceContent: "<p>Editable body</p>",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	})
	db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	})

	resp, err := s.GetProject(project.ID, &owner.ID)
	require.NoError(t, err)
	assert.Equal(t, project.ID, resp.ID)
	assert.Equal(t, models.ProjectRoleOwner, resp.Role)
	assert.Equal(t, "<p>Editable body</p>", resp.SourceContent)
	assert.Len(t, resp.Publications, 1)

	collaboratorResp, err := s.GetProject(project.ID, &editor.ID)
	require.NoError(t, err)
	assert.Equal(t, project.ID, collaboratorResp.ID)
	assert.Equal(t, models.ProjectRoleEditor, collaboratorResp.Role)
	assert.Equal(t, "<p>Editable body</p>", collaboratorResp.SourceContent)

	_, err = s.GetProject(project.ID, &stranger.ID)
	require.ErrorIs(t, err, services.ErrForbidden)
}

func TestUpdateProjectRebuildsSelectedPublications(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	db.Create(&owner)
	db.Create(&stranger)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusPublished,
	}
	db.Create(&project)
	publishedAt := time.Now()
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "wechat",
		Enabled:      true,
		Status:       models.PublicationStatusPublished,
		PublishURL:   "https://example.com/old",
		RemoteID:     "old-remote",
		PublishedAt:  &publishedAt,
		RetryCount:   2,
		ErrorMessage: "old error",
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "zhihu",
		Enabled:      true,
		Status:       models.PublicationStatusFailed,
		ErrorMessage: "failed before",
	})

	resp, err := s.UpdateProject(project.ID, owner.ID, dto.UpdateProjectRequest{
		Title:         "New title",
		SourceContent: "<p>New body</p>",
		Summary:       "New body",
		Platforms:     []string{"zhihu", "douyin"},
	})

	require.NoError(t, err)
	assert.Equal(t, "New title", resp.Title)
	assert.Equal(t, "<p>New body</p>", resp.SourceContent)
	assert.Len(t, resp.Publications, 3)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	assert.Equal(t, "New title", saved.Title)
	assert.Equal(t, "<p>New body</p>", saved.SourceContent)
	assert.Equal(t, models.ProjectStatusReady, saved.Status)

	var wechatPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.False(t, wechatPub.Enabled)
	assert.Equal(t, models.PublicationStatusDisabled, wechatPub.Status)

	var zhihuPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	assert.True(t, zhihuPub.Enabled)
	assert.Equal(t, models.PublicationStatusPending, zhihuPub.Status)
	assert.Empty(t, zhihuPub.ErrorMessage)
	assert.Empty(t, zhihuPub.PublishURL)
	assert.Nil(t, zhihuPub.PublishedAt)

	var douyinPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&douyinPub, "project_id = ? AND platform = ?", project.ID, "douyin").Error)
	assert.True(t, douyinPub.Enabled)
	assert.Equal(t, models.PublicationStatusPending, douyinPub.Status)

	_, err = s.UpdateProject(project.ID, stranger.ID, dto.UpdateProjectRequest{
		Title:         "Not allowed",
		SourceContent: "content",
		Platforms:     []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)
}

func TestUpdateProjectSyncsLinkedCollabDocumentSnapshot(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	initializer := &testsupport.FakeProjectDocumentInitializer{
		SyncProjectSourceContentFunc: func(_ context.Context, documentID uuid.UUID) error {
			return db.Model(&models.Project{}).
				Where("collab_document_id = ?", documentID).
				Update("source_content", "<p>Realtime update snapshot</p>").Error
		},
	}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)

	document := models.CollabDocument{
		OwnerUserID: owner.ID,
		Title:       "Collaborative project",
		Status:      models.CollabDocumentStatusActive,
	}
	require.NoError(t, db.Create(&document).Error)
	require.NoError(t, db.Create(&models.CollabDocumentState{
		DocumentID:        document.ID,
		YDocState:         []byte{1},
		StateVector:       []byte{},
		CompactedUntilSeq: 0,
		StateSizeBytes:    1,
		UpdatedAt:         time.Now(),
	}).Error)
	project := models.Project{
		UserID:           owner.ID,
		CollabDocumentID: &document.ID,
		Title:            "Old title",
		SourceContent:    "<p>Stale canonical content</p>",
		Status:           models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
	}).Error)

	updated, err := s.UpdateProject(project.ID, owner.ID, dto.UpdateProjectRequest{
		Title:         "Updated title",
		SourceContent: "<p>Stale browser payload</p>",
		Platforms:     []string{"wechat"},
	})

	require.NoError(t, err)
	require.Equal(t, "Updated title", updated.Title)
	require.Equal(t, "<p>Realtime update snapshot</p>", updated.SourceContent)
	require.Equal(t, []uuid.UUID{document.ID}, initializer.SourceContentDocumentIDs)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, "Updated title", saved.Title)
	require.Equal(t, "<p>Realtime update snapshot</p>", saved.SourceContent)
	require.Equal(t, models.ProjectStatusReady, saved.Status)
}

func TestUpdateProjectPreservesRequestContentForUninitializedLinkedCollabDocument(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	initializer := &testsupport.FakeProjectDocumentInitializer{
		SyncProjectSourceContentFunc: func(_ context.Context, _ uuid.UUID) error {
			t.Fatal("uninitialized project collab document should not be synced before saving request content")
			return nil
		},
	}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)

	document := models.CollabDocument{
		OwnerUserID: owner.ID,
		Title:       "Collaborative project",
		Status:      models.CollabDocumentStatusActive,
	}
	require.NoError(t, db.Create(&document).Error)
	project := models.Project{
		UserID:           owner.ID,
		CollabDocumentID: &document.ID,
		Title:            "Old title",
		SourceContent:    "<p>Old canonical content</p>",
		Status:           models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
	}).Error)

	updated, err := s.UpdateProject(project.ID, owner.ID, dto.UpdateProjectRequest{
		Title:         "Updated title",
		SourceContent: "<p>Request payload</p>",
		Platforms:     []string{"wechat"},
	})

	require.NoError(t, err)
	require.Equal(t, "Updated title", updated.Title)
	require.Equal(t, "<p>Request payload</p>", updated.SourceContent)
	require.Empty(t, initializer.SourceContentDocumentIDs)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, "<p>Request payload</p>", saved.SourceContent)
	require.Equal(t, models.ProjectStatusReady, saved.Status)
}

func TestUpdateProjectAllowsEditorAndRejectsViewer(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	updated, err := s.UpdateProject(project.ID, editor.ID, dto.UpdateProjectRequest{
		Title:         "Editor title",
		SourceContent: "editor body",
		Platforms:     []string{"zhihu"},
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)
	require.Equal(t, "Editor title", updated.Title)
	require.Equal(t, "editor body", updated.SourceContent)

	var wechatPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.False(t, wechatPub.Enabled)
	require.Equal(t, models.PublicationStatusDisabled, wechatPub.Status)

	var zhihuPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.True(t, zhihuPub.Enabled)
	require.Equal(t, models.PublicationStatusPending, zhihuPub.Status)

	_, err = s.UpdateProject(project.ID, viewer.ID, dto.UpdateProjectRequest{
		Title:         "Viewer title",
		SourceContent: "viewer body",
		Platforms:     []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)
}

func TestSaveProjectContentAllowsEditorAndRejectsViewer(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	updated, err := s.SaveProjectContent(project.ID, editor.ID, dto.SaveProjectContentRequest{
		Title:         "Editor title",
		SourceContent: "editor body",
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)
	require.Equal(t, "Editor title", updated.Title)
	require.Equal(t, "editor body", updated.SourceContent)

	_, err = s.SaveProjectContent(project.ID, viewer.ID, dto.SaveProjectContentRequest{
		Title:         "Viewer title",
		SourceContent: "viewer body",
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, "Editor title", saved.Title)
	require.Equal(t, "editor body", saved.SourceContent)
}

func TestSaveProjectContentSanitizesStoredSourceContent(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "<p>old body</p>",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)

	updated, err := s.SaveProjectContent(project.ID, owner.ID, dto.SaveProjectContentRequest{
		Title: "Saved title",
		SourceContent: `<h2>Safe heading</h2>
			<p onclick="alert(1)">Safe paragraph</p>
			<img src="https://example.com/image.png" onerror="alert(1)" alt="cover">
			<a href="javascript:alert(1)">bad link</a>
			<svg onload="alert(1)"></svg>`,
	})

	require.NoError(t, err)
	require.Contains(t, updated.SourceContent, "<h2>Safe heading</h2>")
	require.Contains(t, updated.SourceContent, `<img src="https://example.com/image.png" alt="cover"/>`)
	assertStoredHTMLHasNoActiveContent(t, updated.SourceContent)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, updated.SourceContent, saved.SourceContent)
}

func TestSaveProjectContentSyncsLinkedCollabDocumentSnapshot(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	initializer := &testsupport.FakeProjectDocumentInitializer{
		SyncProjectSourceContentFunc: func(_ context.Context, documentID uuid.UUID) error {
			return db.Model(&models.Project{}).
				Where("collab_document_id = ?", documentID).
				Update("source_content", "<p>Realtime snapshot</p>").Error
		},
	}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)

	document := models.CollabDocument{
		OwnerUserID: owner.ID,
		Title:       "Collaborative project",
		Status:      models.CollabDocumentStatusActive,
	}
	require.NoError(t, db.Create(&document).Error)
	require.NoError(t, db.Create(&models.CollabDocumentState{
		DocumentID:        document.ID,
		YDocState:         []byte{1},
		StateVector:       []byte{},
		CompactedUntilSeq: 0,
		StateSizeBytes:    1,
		UpdatedAt:         time.Now(),
	}).Error)
	project := models.Project{
		UserID:           owner.ID,
		CollabDocumentID: &document.ID,
		Title:            "Old title",
		SourceContent:    "<p>Stale canonical content</p>",
		Status:           models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)

	updated, err := s.SaveProjectContent(project.ID, owner.ID, dto.SaveProjectContentRequest{
		Title:         "Saved title",
		SourceContent: "<p>Stale browser payload</p>",
	})

	require.NoError(t, err)
	require.Equal(t, "Saved title", updated.Title)
	require.Equal(t, "<p>Realtime snapshot</p>", updated.SourceContent)
	require.Equal(t, []uuid.UUID{document.ID}, initializer.SourceContentDocumentIDs)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, "Saved title", saved.Title)
	require.Equal(t, "<p>Realtime snapshot</p>", saved.SourceContent)
	require.Equal(t, models.ProjectStatusReady, saved.Status)
}

func TestSaveProjectContentPreservesRequestContentForUninitializedLinkedCollabDocument(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	initializer := &testsupport.FakeProjectDocumentInitializer{
		SyncProjectSourceContentFunc: func(_ context.Context, _ uuid.UUID) error {
			t.Fatal("uninitialized project collab document should not be synced before saving request content")
			return nil
		},
	}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)

	document := models.CollabDocument{
		OwnerUserID: owner.ID,
		Title:       "Collaborative project",
		Status:      models.CollabDocumentStatusActive,
	}
	require.NoError(t, db.Create(&document).Error)
	project := models.Project{
		UserID:           owner.ID,
		CollabDocumentID: &document.ID,
		Title:            "Old title",
		SourceContent:    "<p>Old canonical content</p>",
		Status:           models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)

	updated, err := s.SaveProjectContent(project.ID, owner.ID, dto.SaveProjectContentRequest{
		Title:         "Saved title",
		SourceContent: "<p>Request payload</p>",
	})

	require.NoError(t, err)
	require.Equal(t, "Saved title", updated.Title)
	require.Equal(t, "<p>Request payload</p>", updated.SourceContent)
	require.Empty(t, initializer.SourceContentDocumentIDs)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, "<p>Request payload</p>", saved.SourceContent)
	require.Equal(t, models.ProjectStatusReady, saved.Status)
}

func assertStoredHTMLHasNoActiveContent(t *testing.T, html string) {
	t.Helper()

	lower := strings.ToLower(html)
	require.NotContains(t, lower, "<script")
	require.NotContains(t, lower, "<svg")
	require.NotContains(t, lower, "javascript:")
	require.NotContains(t, lower, "onclick")
	require.NotContains(t, lower, "onerror")
}

func TestSaveProjectPlatformsAllowsEditorAndRejectsViewer(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Draft title",
		SourceContent: "draft body",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusAdapted,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	updated, err := s.SaveProjectPlatforms(project.ID, editor.ID, dto.SaveProjectPlatformsRequest{
		Platforms: []string{"wechat", "zhihu"},
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)

	var zhihuPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.True(t, zhihuPub.Enabled)
	require.Equal(t, models.PublicationStatusPending, zhihuPub.Status)

	_, err = s.SaveProjectPlatforms(project.ID, viewer.ID, dto.SaveProjectPlatformsRequest{
		Platforms: []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)
}
