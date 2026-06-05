package mediaasset_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage/fake"
	"github.com/kurodakayn/mpp-backend/internal/services/mediaasset"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestCreateMediaUploadCreatesPendingAssetAndPresignsPut(t *testing.T) {
	db, service, _ := setupMediaAssetService(t)
	owner, project := createMediaAssetProject(t, db)

	resp, err := service.CreateProjectMediaUpload(project.ID, owner.ID, dto.CreateMediaUploadRequest{
		Filename:  "hero.png",
		MimeType:  "image/png",
		SizeBytes: 11,
		Usage:     models.MediaAssetUsageEditorImage,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, resp.AssetID)
	require.Equal(t, "mpp://media/"+resp.AssetID.String(), resp.ObjectRef)
	require.Equal(t, "fake://put/mpp-media/"+mediaAssetObjectKey(t, db, resp.AssetID), resp.UploadURL)
	require.Equal(t, map[string]string{"Content-Type": "image/png"}, resp.Headers)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), resp.ExpiresAt, 2*time.Second)

	var asset models.MediaAsset
	require.NoError(t, db.First(&asset, "id = ?", resp.AssetID).Error)
	require.Equal(t, owner.ID, asset.UserID)
	require.NotNil(t, asset.ProjectID)
	require.Equal(t, project.ID, *asset.ProjectID)
	require.NotNil(t, asset.WorkspaceID)
	require.Equal(t, *project.WorkspaceID, *asset.WorkspaceID)
	require.Equal(t, "mpp-media", asset.Bucket)
	require.Equal(t, models.MediaAssetStatusPending, asset.Status)
	require.Equal(t, models.MediaAssetUsageEditorImage, asset.Usage)
	require.Equal(t, "image/png", asset.MimeType)
	require.EqualValues(t, 11, asset.SizeBytes)
	require.True(t, strings.HasSuffix(asset.ObjectKey, "/hero.png"))
}

func TestCompleteMediaUploadMarksReadyAfterObjectExists(t *testing.T) {
	db, service, storage := setupMediaAssetService(t)
	owner, project := createMediaAssetProject(t, db)
	upload, err := service.CreateProjectMediaUpload(project.ID, owner.ID, dto.CreateMediaUploadRequest{
		Filename:  "hero.png",
		MimeType:  "image/png",
		SizeBytes: 11,
		Usage:     models.MediaAssetUsageEditorImage,
	})
	require.NoError(t, err)
	key := mediaAssetObjectKey(t, db, upload.AssetID)
	storage.StoreObject(key, []byte("image-bytes"), objectstorage.ObjectInfo{
		Key:         key,
		ContentType: "image/png",
		Size:        11,
		ETag:        "etag-value",
	})

	resp, err := service.CompleteMediaUpload(upload.AssetID, owner.ID)

	require.NoError(t, err)
	require.Equal(t, upload.AssetID, resp.AssetID)
	require.Equal(t, models.MediaAssetStatusReady, resp.Status)

	var asset models.MediaAsset
	require.NoError(t, db.First(&asset, "id = ?", upload.AssetID).Error)
	require.Equal(t, models.MediaAssetStatusReady, asset.Status)
	require.Equal(t, "etag-value", asset.ETag)
	require.Empty(t, asset.ErrorMessage)
}

func TestResolveMediaAssetsPresignsReadyAssets(t *testing.T) {
	db, service, storage := setupMediaAssetService(t)
	owner, project := createMediaAssetProject(t, db)
	upload, err := service.CreateProjectMediaUpload(project.ID, owner.ID, dto.CreateMediaUploadRequest{
		Filename:  "hero.png",
		MimeType:  "image/png",
		SizeBytes: 11,
		Usage:     models.MediaAssetUsageEditorImage,
	})
	require.NoError(t, err)
	stagingKey := mediaAssetObjectKey(t, db, upload.AssetID)
	storage.StoreObject(stagingKey, []byte("image-bytes"), objectstorage.ObjectInfo{
		Key:         stagingKey,
		ContentType: "image/png",
		Size:        11,
		ETag:        "etag-value",
	})
	_, err = service.CompleteMediaUpload(upload.AssetID, owner.ID)
	require.NoError(t, err)
	finalKey := mediaAssetObjectKey(t, db, upload.AssetID)

	resp, err := service.ResolveMediaAssets(owner.ID, dto.ResolveMediaAssetsRequest{
		AssetIDs: []uuid.UUID{upload.AssetID},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, upload.AssetID, resp.Items[0].AssetID)
	require.Equal(t, "fake://get/mpp-media/"+finalKey, resp.Items[0].URL)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), resp.Items[0].ExpiresAt, 2*time.Second)
}

func TestCompleteMediaUploadPromotesStagingObjectToFinalKey(t *testing.T) {
	db, service, storage := setupMediaAssetService(t)
	owner, project := createMediaAssetProject(t, db)
	upload, err := service.CreateProjectMediaUpload(project.ID, owner.ID, dto.CreateMediaUploadRequest{
		Filename:  "hero.png",
		MimeType:  "image/png",
		SizeBytes: 11,
		Usage:     models.MediaAssetUsageEditorImage,
	})
	require.NoError(t, err)
	stagingKey := mediaAssetObjectKey(t, db, upload.AssetID)
	storage.StoreObject(stagingKey, []byte("image-bytes"), objectstorage.ObjectInfo{
		Key:         stagingKey,
		ContentType: "image/png",
		Size:        11,
		ETag:        "original-etag",
	})

	_, err = service.CompleteMediaUpload(upload.AssetID, owner.ID)
	require.NoError(t, err)
	finalKey := mediaAssetObjectKey(t, db, upload.AssetID)
	require.NotEqual(t, stagingKey, finalKey)

	storage.StoreObject(stagingKey, []byte("mutated-image-bytes"), objectstorage.ObjectInfo{
		Key:         stagingKey,
		ContentType: "image/png",
		Size:        19,
		ETag:        "mutated-etag",
	})

	finalInfo, err := storage.HeadObject(context.Background(), finalKey)
	require.NoError(t, err)
	require.Equal(t, "original-etag", finalInfo.ETag)
	require.EqualValues(t, 11, finalInfo.Size)

	resp, err := service.ResolveMediaAssets(owner.ID, dto.ResolveMediaAssetsRequest{
		AssetIDs: []uuid.UUID{upload.AssetID},
	})
	require.NoError(t, err)
	require.Equal(t, "fake://get/mpp-media/"+finalKey, resp.Items[0].URL)
}

func TestViewerCannotCreateMediaUpload(t *testing.T) {
	db, service, _ := setupMediaAssetService(t)
	owner, project := createMediaAssetProject(t, db)
	viewer := models.User{
		ID:           uuid.New(),
		Username:     "viewer",
		Email:        "viewer@example.com",
		PasswordHash: "hash",
		Role:         "user",
	}
	require.NoError(t, db.Create(&viewer).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	_, err := service.CreateProjectMediaUpload(project.ID, viewer.ID, dto.CreateMediaUploadRequest{
		Filename:  "hero.png",
		MimeType:  "image/png",
		SizeBytes: 11,
		Usage:     models.MediaAssetUsageEditorImage,
	})

	require.ErrorIs(t, err, mediaasset.ErrForbidden)
}

func TestDeleteMediaAssetSoftDeletesRecordAndObject(t *testing.T) {
	db, service, storage := setupMediaAssetService(t)
	owner, project := createMediaAssetProject(t, db)
	upload, err := service.CreateProjectMediaUpload(project.ID, owner.ID, dto.CreateMediaUploadRequest{
		Filename:  "hero.png",
		MimeType:  "image/png",
		SizeBytes: 11,
		Usage:     models.MediaAssetUsageEditorImage,
	})
	require.NoError(t, err)
	key := mediaAssetObjectKey(t, db, upload.AssetID)
	storage.StoreObject(key, []byte("image-bytes"), objectstorage.ObjectInfo{
		Key:         key,
		ContentType: "image/png",
		Size:        11,
		ETag:        "etag-value",
	})

	require.NoError(t, service.DeleteMediaAsset(upload.AssetID, owner.ID))

	_, err = storage.HeadObject(context.Background(), key)
	require.ErrorIs(t, err, objectstorage.ErrObjectNotFound)

	var asset models.MediaAsset
	require.NoError(t, db.Unscoped().First(&asset, "id = ?", upload.AssetID).Error)
	require.Equal(t, models.MediaAssetStatusDeleted, asset.Status)
	require.True(t, asset.DeletedAt.Valid)
}

func setupMediaAssetService(t *testing.T) (*gorm.DB, *mediaasset.Service, *fake.Client) {
	t.Helper()

	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.MediaAsset{}))

	storage := fake.NewClient()
	service := mediaasset.NewService(db, projectsvc.NewService(db))
	service.UseObjectStorage(storage, objectstorage.Config{
		Enabled:        true,
		Provider:       objectstorage.ProviderR2,
		Bucket:         "mpp-media",
		UploadURLTTL:   10 * time.Minute,
		DownloadURLTTL: 5 * time.Minute,
	})
	return db, service, storage
}

func createMediaAssetProject(t *testing.T, db *gorm.DB) (models.User, models.Project) {
	t.Helper()

	owner := models.User{
		ID:           uuid.New(),
		Username:     "owner",
		Email:        "owner@example.com",
		PasswordHash: "hash",
		Role:         "user",
	}
	require.NoError(t, db.Create(&owner).Error)

	workspaceID := models.PersonalWorkspaceID(owner.ID)
	project := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspaceID,
		Title:         "Project",
		SourceContent: "<p>Hello</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	return owner, project
}

func mediaAssetObjectKey(t *testing.T, db *gorm.DB, assetID uuid.UUID) string {
	t.Helper()

	var asset models.MediaAsset
	err := db.First(&asset, "id = ?", assetID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected media asset %s to exist", assetID)
	}
	require.NoError(t, err)
	return asset.ObjectKey
}
