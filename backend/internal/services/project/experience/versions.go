package experience

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	"github.com/kurodakayn/mpp-backend/internal/services/project/publicationselection"
)

func (s *Service) ListProjectVersions(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectVersionsResponse, error) {
	project, err := s.accessibleProject(projectID, userID)
	if err != nil {
		return nil, err
	}

	var versions []models.ProjectVersion
	if err := s.db.
		Preload("Creator", selectUserIdentity).
		Where("workspace_id = ? AND project_id = ?", models.ProjectWorkspaceID(project), projectID).
		Order("version_number desc").
		Find(&versions).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectVersion, 0, len(versions))
	for _, version := range versions {
		items = append(items, projectVersionFromModel(version))
	}
	return &dto.ProjectVersionsResponse{Items: items}, nil
}

func (s *Service) RestoreProjectVersion(projectID uuid.UUID, userID uuid.UUID, versionID uuid.UUID) (*dto.RestoreProjectVersionResponse, error) {
	if versionID == uuid.Nil {
		return nil, ErrInvalidProjectVersion
	}
	var version models.ProjectVersion
	if err := s.db.Preload("Creator", selectUserIdentity).First(&version, "id = ? AND project_id = ?", versionID, projectID).Error; err != nil {
		return nil, err
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		role, err := accesspolicy.ProjectAccessRoleWithDB(tx, project, userID)
		if err != nil {
			return err
		}
		if !accesspolicy.CanEditProjectRole(role) {
			return accesspolicy.ErrForbidden
		}
		previousCollabDocumentID := project.CollabDocumentID
		if err := tx.Model(&project).Updates(map[string]any{
			"title":              version.Title,
			"source_content":     version.SourceContent,
			"status":             models.ProjectStatusReady,
			"collab_document_id": nil,
		}).Error; err != nil {
			return err
		}
		project.Title = version.Title
		project.SourceContent = version.SourceContent
		project.Status = models.ProjectStatusReady
		project.CollabDocumentID = nil
		if err := CreateProjectVersion(tx, project, userID, "version_restore"); err != nil {
			return err
		}
		if err := publicationselection.MarkDraftsStale(tx, project.ID); err != nil {
			return err
		}
		var publications []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&publications).Error; err != nil {
			return err
		}
		if err := s.refreshProjectMediaUsages(tx, project, publications); err != nil {
			return err
		}
		return RecordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityVersionRestored, map[string]any{
			"detached_collab_document_id": uuidString(previousCollabDocumentID),
			"version_id":                  version.ID.String(),
			"version_number":              version.VersionNumber,
		})
	}); err != nil {
		return nil, err
	}

	project, err := s.getProject(projectID, &userID)
	if err != nil {
		return nil, err
	}
	versionDTO := projectVersionFromModel(version)
	return &dto.RestoreProjectVersionResponse{Project: project, Version: versionDTO}, nil
}

func CreateProjectVersion(tx *gorm.DB, project models.Project, userID uuid.UUID, source string) error {
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		First(&models.Project{}, "id = ?", project.ID).Error; err != nil {
		return err
	}

	var latestVersionNumber int
	if err := tx.
		Model(&models.ProjectVersion{}).
		Select("COALESCE(MAX(version_number), 0)").
		Where("project_id = ?", project.ID).
		Scan(&latestVersionNumber).Error; err != nil {
		return err
	}
	collabSeq := int64(0)
	if project.CollabDocumentID != nil && *project.CollabDocumentID != uuid.Nil {
		var document models.CollabDocument
		if err := tx.Select("current_seq").First(&document, "id = ?", *project.CollabDocumentID).Error; err == nil {
			collabSeq = document.CurrentSeq
		}
	}
	return tx.Create(&models.ProjectVersion{
		WorkspaceID:      models.ProjectWorkspaceID(project),
		ProjectID:        project.ID,
		CreatedBy:        userID,
		VersionNumber:    latestVersionNumber + 1,
		Title:            project.Title,
		SourceContent:    project.SourceContent,
		CollabDocumentID: project.CollabDocumentID,
		CollabSeq:        collabSeq,
		Source:           source,
	}).Error
}

func projectVersionFromModel(version models.ProjectVersion) dto.ProjectVersion {
	return dto.ProjectVersion{
		ID:               version.ID,
		ProjectID:        version.ProjectID,
		CreatedBy:        version.CreatedBy,
		CreatorUsername:  version.Creator.Username,
		CreatorEmail:     version.Creator.Email,
		VersionNumber:    version.VersionNumber,
		Title:            version.Title,
		Source:           version.Source,
		CollabDocumentID: version.CollabDocumentID,
		CollabSeq:        version.CollabSeq,
		CreatedAt:        version.CreatedAt,
	}
}
