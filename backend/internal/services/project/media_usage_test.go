package project

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestUpsertMediaUsagesIgnoresDuplicateAssetResourceRefs(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.MediaAsset{}))

	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	workspace := models.Workspace{
		ID:          models.PersonalWorkspaceID(user.ID),
		OwnerUserID: user.ID,
		Name:        models.PersonalWorkspaceName,
		Slug:        "personal-" + user.ID.String(),
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)
	asset := models.MediaAsset{
		ID:               uuid.New(),
		UserID:           user.ID,
		WorkspaceID:      &workspace.ID,
		Bucket:           "mpp-media",
		ObjectKey:        "workspaces/" + workspace.ID.String() + "/library/hero.png",
		OriginalFilename: "hero.png",
		MimeType:         "image/png",
		SizeBytes:        12,
		Usage:            models.MediaAssetUsageEditorImage,
		LibraryScope:     models.MediaAssetLibraryScopeWorkspace,
		Status:           models.MediaAssetStatusReady,
	}
	require.NoError(t, db.Create(&asset).Error)
	resourceID := uuid.New()

	require.NoError(t, upsertMediaUsages(db, workspace.ID, nil, nil, nil, "template", resourceID, models.MediaAssetUsageEditorImage, []uuid.UUID{asset.ID}))
	require.NoError(t, upsertMediaUsages(db, workspace.ID, nil, nil, nil, "template", resourceID, models.MediaAssetUsageCoverImage, []uuid.UUID{asset.ID}))

	var count int64
	require.NoError(t, db.Model(&models.MediaAssetUsage{}).
		Where("media_asset_id = ? AND resource_type = ? AND resource_id = ?", asset.ID, "template", resourceID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}
