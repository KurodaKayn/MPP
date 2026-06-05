package mediaasset

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

const (
	defaultMediaUploadMaxBytes = 5 * 1024 * 1024
	mediaObjectRefPrefix       = "mpp://media/"
)

var allowedMediaAssetMIMETypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var allowedMediaAssetUsages = map[string]struct{}{
	models.MediaAssetUsageCoverImage:  {},
	models.MediaAssetUsageEditorImage: {},
}

func (s *Service) CreateProjectMediaUpload(projectID uuid.UUID, userID uuid.UUID, req dto.CreateMediaUploadRequest) (*dto.CreateMediaUploadResponse, error) {
	if err := s.ensureMediaStorage(); err != nil {
		return nil, err
	}
	filename := strings.TrimSpace(req.Filename)
	mimeType := strings.ToLower(strings.TrimSpace(req.MimeType))
	usage := strings.TrimSpace(req.Usage)
	if err := validateMediaUploadRequest(filename, mimeType, req.SizeBytes, usage); err != nil {
		return nil, err
	}

	var project models.Project
	if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.projects.ProjectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !projectsvc.CanEditProjectRole(role) {
		return nil, ErrForbidden
	}

	workspaceID := mediaAssetWorkspaceID(project)
	asset := models.MediaAsset{
		ID:               uuid.New(),
		UserID:           userID,
		WorkspaceID:      &workspaceID,
		ProjectID:        &project.ID,
		Bucket:           s.storageConfig.Bucket,
		ObjectKey:        mediaAssetObjectKey(workspaceID, project.ID, uuid.Nil, filename),
		OriginalFilename: filename,
		MimeType:         mimeType,
		SizeBytes:        req.SizeBytes,
		Usage:            usage,
		Status:           models.MediaAssetStatusPending,
	}
	asset.ObjectKey = mediaAssetObjectKey(workspaceID, project.ID, asset.ID, filename)
	if err := s.db.Create(&asset).Error; err != nil {
		return nil, err
	}

	presigned, err := s.objectStorage.PresignPutObject(s.requestContext(), objectstorage.PutObjectInput{
		Bucket:      asset.Bucket,
		Key:         asset.ObjectKey,
		ContentType: asset.MimeType,
		Expires:     s.storageConfig.UploadURLTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("presign media upload: %w", err)
	}

	return &dto.CreateMediaUploadResponse{
		AssetID:   asset.ID,
		ObjectRef: mediaAssetObjectRef(asset.ID),
		UploadURL: presigned.URL,
		Headers:   presigned.Headers,
		ExpiresAt: time.Now().UTC().Add(presigned.Expires),
	}, nil
}

func (s *Service) CompleteMediaUpload(assetID uuid.UUID, userID uuid.UUID) (*dto.CompleteMediaUploadResponse, error) {
	if err := s.ensureMediaStorage(); err != nil {
		return nil, err
	}

	asset, err := s.mediaAssetForEdit(assetID, userID)
	if err != nil {
		return nil, err
	}
	info, err := s.objectStorage.HeadObject(s.requestContext(), asset.ObjectKey)
	if err != nil {
		if errors.Is(err, objectstorage.ErrObjectNotFound) {
			if updateErr := s.markMediaAssetFailed(asset, "uploaded object was not found"); updateErr != nil {
				return nil, updateErr
			}
			return nil, ErrMediaAssetUploadIncomplete
		}
		return nil, err
	}
	if info.Size != asset.SizeBytes || !strings.EqualFold(info.ContentType, asset.MimeType) {
		if updateErr := s.markMediaAssetFailed(asset, "uploaded object metadata does not match"); updateErr != nil {
			return nil, updateErr
		}
		return nil, ErrInvalidMediaAsset
	}

	if err := s.db.Model(asset).Updates(map[string]interface{}{
		"e_tag":         info.ETag,
		"error_message": "",
		"size_bytes":    info.Size,
		"status":        models.MediaAssetStatusReady,
	}).Error; err != nil {
		return nil, err
	}

	return &dto.CompleteMediaUploadResponse{
		AssetID:   asset.ID,
		ObjectRef: mediaAssetObjectRef(asset.ID),
		Status:    models.MediaAssetStatusReady,
	}, nil
}

func (s *Service) ResolveMediaAssets(userID uuid.UUID, req dto.ResolveMediaAssetsRequest) (*dto.ResolveMediaAssetsResponse, error) {
	if err := s.ensureMediaStorage(); err != nil {
		return nil, err
	}
	if len(req.AssetIDs) == 0 {
		return nil, ErrInvalidMediaAsset
	}

	items := make([]dto.ResolvedMediaAsset, 0, len(req.AssetIDs))
	for _, assetID := range req.AssetIDs {
		asset, err := s.mediaAssetForRead(assetID, userID)
		if err != nil {
			return nil, err
		}
		if asset.Status != models.MediaAssetStatusReady {
			return nil, ErrMediaAssetNotReady
		}
		presigned, err := s.objectStorage.PresignGetObject(s.requestContext(), objectstorage.GetObjectInput{
			Bucket:  asset.Bucket,
			Key:     asset.ObjectKey,
			Expires: s.storageConfig.DownloadURLTTL,
		})
		if err != nil {
			return nil, fmt.Errorf("presign media download: %w", err)
		}
		items = append(items, dto.ResolvedMediaAsset{
			AssetID:   asset.ID,
			URL:       presigned.URL,
			ExpiresAt: time.Now().UTC().Add(presigned.Expires),
		})
	}
	return &dto.ResolveMediaAssetsResponse{Items: items}, nil
}

func (s *Service) DeleteMediaAsset(assetID uuid.UUID, userID uuid.UUID) error {
	if err := s.ensureMediaStorage(); err != nil {
		return err
	}
	asset, err := s.mediaAssetForEdit(assetID, userID)
	if err != nil {
		return err
	}
	if err := s.objectStorage.DeleteObject(s.requestContext(), asset.ObjectKey); err != nil && !errors.Is(err, objectstorage.ErrObjectNotFound) {
		return err
	}
	if err := s.db.Model(asset).Update("status", models.MediaAssetStatusDeleted).Error; err != nil {
		return err
	}
	return s.db.Delete(asset).Error
}

func (s *Service) ensureMediaStorage() error {
	if s.objectStorage == nil || !s.storageConfig.Enabled || strings.TrimSpace(s.storageConfig.Bucket) == "" {
		return ErrMediaStorageUnavailable
	}
	if s.storageConfig.UploadURLTTL <= 0 || s.storageConfig.DownloadURLTTL <= 0 {
		return ErrMediaStorageUnavailable
	}
	return nil
}

func validateMediaUploadRequest(filename string, mimeType string, sizeBytes int64, usage string) error {
	if filename == "" || mimeType == "" || usage == "" {
		return ErrInvalidMediaAsset
	}
	if sizeBytes <= 0 || sizeBytes > defaultMediaUploadMaxBytes {
		return ErrInvalidMediaAsset
	}
	if _, ok := allowedMediaAssetMIMETypes[mimeType]; !ok {
		return ErrInvalidMediaAsset
	}
	if _, ok := allowedMediaAssetUsages[usage]; !ok {
		return ErrInvalidMediaAsset
	}
	return nil
}

func (s *Service) mediaAssetForEdit(assetID uuid.UUID, userID uuid.UUID) (*models.MediaAsset, error) {
	asset, project, err := s.mediaAssetWithProject(assetID)
	if err != nil {
		return nil, err
	}
	role, err := s.projects.ProjectAccessRole(*project, userID)
	if err != nil {
		return nil, err
	}
	if !projectsvc.CanEditProjectRole(role) {
		return nil, ErrForbidden
	}
	return asset, nil
}

func (s *Service) mediaAssetForRead(assetID uuid.UUID, userID uuid.UUID) (*models.MediaAsset, error) {
	asset, project, err := s.mediaAssetWithProject(assetID)
	if err != nil {
		return nil, err
	}
	if _, err := s.projects.ProjectAccessRole(*project, userID); err != nil {
		return nil, err
	}
	return asset, nil
}

func (s *Service) mediaAssetWithProject(assetID uuid.UUID) (*models.MediaAsset, *models.Project, error) {
	if assetID == uuid.Nil {
		return nil, nil, ErrInvalidMediaAsset
	}
	var asset models.MediaAsset
	if err := s.db.First(&asset, "id = ?", assetID).Error; err != nil {
		return nil, nil, err
	}
	if asset.ProjectID == nil || *asset.ProjectID == uuid.Nil {
		return nil, nil, ErrInvalidMediaAsset
	}
	var project models.Project
	if err := s.db.First(&project, "id = ?", *asset.ProjectID).Error; err != nil {
		return nil, nil, err
	}
	return &asset, &project, nil
}

func (s *Service) markMediaAssetFailed(asset *models.MediaAsset, message string) error {
	return s.db.Model(asset).Updates(map[string]interface{}{
		"error_message": message,
		"status":        models.MediaAssetStatusFailed,
	}).Error
}

func mediaAssetWorkspaceID(project models.Project) uuid.UUID {
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return *project.WorkspaceID
	}
	return models.PersonalWorkspaceID(project.UserID)
}

func mediaAssetObjectKey(workspaceID uuid.UUID, projectID uuid.UUID, assetID uuid.UUID, filename string) string {
	return path.Join(
		"workspaces",
		workspaceID.String(),
		"projects",
		projectID.String(),
		"assets",
		assetID.String(),
		safeMediaFilename(filename),
	)
}

func safeMediaFilename(filename string) string {
	name := strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")
	name = path.Base(name)
	if name == "." || name == "/" || name == "" {
		return "asset"
	}

	var b strings.Builder
	lastHyphen := false
	for _, r := range name {
		allowed := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_'
		if allowed {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	result := strings.Trim(b.String(), ".-_")
	if result == "" {
		return "asset"
	}
	return result
}

func mediaAssetObjectRef(assetID uuid.UUID) string {
	return mediaObjectRefPrefix + assetID.String()
}
