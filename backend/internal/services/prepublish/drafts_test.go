package prepublish_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestSyncProjectPrepublishGeneratesPlatformDrafts(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	compiler := &testsupport.FakeProjectDraftCompiler{}
	s.SetDraftCompiler(compiler)

	owner := models.User{Username: "owner"}
	db.Create(&owner)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`,
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "zhihu",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "x",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "douyin",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"wechat", "zhihu", "x", "douyin"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	assert.NoError(t, err)
	assert.Equal(t, project.ID, resp.ProjectID)
	assert.Len(t, resp.Items, 4)
	assert.Equal(t, []string{"wechat", "zhihu", "x", "douyin"}, compiler.LastPlatforms)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.Equal(t, models.PublicationStatusAdapted, wechatPub.Status)

	var wechatContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &wechatContent))
	assert.Equal(t, "html", wechatContent["format"])
	assert.Equal(t, `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`, wechatContent["html"])

	var zhihuPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	assert.Equal(t, models.PublicationStatusAdapted, zhihuPub.Status)

	var zhihuContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(zhihuPub.AdaptedContent, &zhihuContent))
	assert.Equal(t, "markdown", zhihuContent["format"])
	assert.Contains(t, zhihuContent["markdown"], "## Heading")
	assert.Contains(t, zhihuContent["markdown"], "**draft**")

	var xPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&xPub, "project_id = ? AND platform = ?", project.ID, "x").Error)
	assert.Equal(t, models.PublicationStatusAdapted, xPub.Status)

	var xContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(xPub.AdaptedContent, &xContent))
	assert.Equal(t, "text", xContent["format"])
	assert.Contains(t, xContent["text"], "Platform title")
	assert.Contains(t, xContent["text"], "Hello draft")

	var douyinPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&douyinPub, "project_id = ? AND platform = ?", project.ID, "douyin").Error)
	assert.Equal(t, models.PublicationStatusAdapted, douyinPub.Status)

	var douyinContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(douyinPub.AdaptedContent, &douyinContent))
	assert.Equal(t, "text", douyinContent["format"])
	assert.Contains(t, douyinContent["text"], "Hello draft")
}

func TestSyncProjectPrepublishReadsLatestCollabSnapshot(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	initializer := &testsupport.FakeProjectDocumentInitializer{
		SyncProjectSourceContentFunc: func(_ context.Context, documentID uuid.UUID) error {
			return db.Model(&models.Project{}).
				Where("collab_document_id = ?", documentID).
				Update("source_content", "<p>Realtime draft</p>").Error
		},
	}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)
	compiler := &testsupport.FakeProjectDraftCompiler{}
	s.SetDraftCompiler(compiler)

	owner := models.User{Username: "owner"}
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
		Title:            "Platform title",
		SourceContent:    "<p>Stale source</p>",
		Status:           models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	}).Error)

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"wechat"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, []uuid.UUID{document.ID}, initializer.SourceContentDocumentIDs)
	require.NotNil(t, compiler.LastProject)
	require.Equal(t, "<p>Realtime draft</p>", compiler.LastProject.SourceContent)

	var wechatPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	var wechatContent map[string]interface{}
	require.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &wechatContent))
	require.Equal(t, "html", wechatContent["format"])
	require.Equal(t, "<p>Realtime draft</p>", wechatContent["html"])
}

func TestSyncProjectPrepublishMarksFailedWhenContentPipelineCompilerFails(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	s.SetDraftCompiler(&testsupport.FakeProjectDraftCompiler{Err: fmt.Errorf("content pipeline unavailable")})

	owner := models.User{Username: "owner"}
	require.NoError(t, db.Create(&owner).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`,
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Platform title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}).Error)

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"zhihu"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, models.PublicationStatusFailed, resp.Items[0].Status)
	require.Contains(t, resp.Items[0].ErrorMessage, "content pipeline unavailable")

	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.Equal(t, models.PublicationStatusFailed, publication.Status)
	require.JSONEq(t, `{}`, string(publication.AdaptedContent))
}

func TestSyncProjectPrepublishRejectsActivePublishWithoutMarkingSyncing(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	compiler := &testsupport.FakeProjectDraftCompiler{}
	s.SetDraftCompiler(compiler)

	owner := models.User{Username: "owner"}
	require.NoError(t, db.Create(&owner).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<p>Hello draft</p>`,
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	lastAttemptAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         datatypes.JSON(`{"title":"Platform title"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
		RemoteID:       "active-remote",
		PublishURL:     "https://example.com/active",
		LastAttemptAt:  &lastAttemptAt,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Platform title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}).Error)

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"wechat", "zhihu"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	require.ErrorIs(t, err, services.ErrPublicationAlreadyPublishing)
	require.Nil(t, resp)
	require.Empty(t, compiler.LastPlatforms)

	var activePublication models.ProjectPlatformPublication
	require.NoError(t, db.First(&activePublication, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.Equal(t, models.PublicationStatusPublishing, activePublication.Status)
	require.Equal(t, "active-remote", activePublication.RemoteID)
	require.Equal(t, "https://example.com/active", activePublication.PublishURL)
	require.NotNil(t, activePublication.LastAttemptAt)
	require.True(t, activePublication.LastAttemptAt.Equal(lastAttemptAt))

	var pendingPublication models.ProjectPlatformPublication
	require.NoError(t, db.First(&pendingPublication, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.Equal(t, models.PublicationStatusPending, pendingPublication.Status)
}

func TestSyncProjectPrepublishDoesNotApplyDraftWhenPublicationBecomesPublishing(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner"}
	require.NoError(t, db.Create(&owner).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<p>Updated draft</p>`,
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	lastAttemptAt := time.Now().UTC()
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Platform title"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"old draft"}`),
		RemoteID:       "active-remote",
		PublishURL:     "https://example.com/active",
	}
	require.NoError(t, db.Create(&publication).Error)

	compiler := &testsupport.FakeProjectDraftCompiler{
		BeforeReturn: func() {
			require.NoError(t, db.Model(&models.ProjectPlatformPublication{}).
				Where("id = ?", publication.ID).
				Updates(map[string]interface{}{
					"last_attempt_at": &lastAttemptAt,
					"status":          models.PublicationStatusPublishing,
				}).Error)
		},
	}
	s.SetDraftCompiler(compiler)

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"wechat"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	require.ErrorIs(t, err, services.ErrPublicationAlreadyPublishing)
	require.Nil(t, resp)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", publication.ID).Error)
	require.Equal(t, models.PublicationStatusPublishing, saved.Status)
	require.Equal(t, datatypes.JSON(`{"format":"html","html":"old draft"}`), saved.AdaptedContent)
	require.Equal(t, "active-remote", saved.RemoteID)
	require.Equal(t, "https://example.com/active", saved.PublishURL)
	require.NotNil(t, saved.LastAttemptAt)
	require.True(t, saved.LastAttemptAt.Equal(lastAttemptAt))
}
