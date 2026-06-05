package dashboard_test

import (
	"encoding/json"
	"fmt"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"testing"
)

func TestSyncProjectPrepublishGeneratesPlatformDrafts(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	compiler := &fakeProjectDraftCompiler{}
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
	assert.Equal(t, []string{"wechat", "zhihu", "x", "douyin"}, compiler.lastPlatforms)

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

func TestSyncProjectPrepublishMarksFailedWhenContentPipelineCompilerFails(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	s.SetDraftCompiler(&fakeProjectDraftCompiler{err: fmt.Errorf("content pipeline unavailable")})

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
