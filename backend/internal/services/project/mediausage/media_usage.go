package mediausage

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/project/projecterr"
)

const objectRefPrefix = "mpp://media/"

var objectRefPattern = regexp.MustCompile(`mpp://media/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

func RefreshProject(tx *gorm.DB, project models.Project, publications []models.ProjectPlatformPublication) error {
	workspaceID := projectWorkspaceID(project)
	if err := tx.Where("project_id = ?", project.ID).Delete(&models.MediaAssetUsage{}).Error; err != nil {
		return err
	}

	sourceRefs := collectAssetIDs(project.SourceContent)
	if err := upsert(tx, workspaceID, &project.ID, nil, nil, "project", project.ID, models.MediaAssetUsageEditorImage, sourceRefs); err != nil {
		return err
	}
	for _, publication := range publications {
		refs := collectAssetIDs(string(publication.Config), string(publication.AdaptedContent))
		if len(refs) == 0 {
			continue
		}
		if err := upsert(tx, workspaceID, &project.ID, &publication.ID, nil, "publication", publication.ID, publication.Platform, refs); err != nil {
			return err
		}
	}
	return nil
}

func RefreshContentTemplate(tx *gorm.DB, workspaceID uuid.UUID, template models.ContentTemplate) error {
	if template.ID == uuid.Nil {
		return projecterr.ErrInvalidProject
	}
	if err := tx.Where("template_id = ?", template.ID).Delete(&models.MediaAssetUsage{}).Error; err != nil {
		return err
	}

	refs := collectAssetIDs(template.SourceTemplate, string(template.PlatformConfig))
	return upsert(tx, workspaceID, nil, nil, &template.ID, "template", template.ID, models.MediaAssetUsageEditorImage, refs)
}

func upsert(tx *gorm.DB, workspaceID uuid.UUID, projectID *uuid.UUID, publicationID *uuid.UUID, templateID *uuid.UUID, resourceType string, resourceID uuid.UUID, usageKind string, assetIDs []uuid.UUID) error {
	for _, assetID := range assetIDs {
		usage := models.MediaAssetUsage{
			MediaAssetID:  assetID,
			WorkspaceID:   workspaceID,
			ProjectID:     projectID,
			PublicationID: publicationID,
			TemplateID:    templateID,
			ResourceType:  resourceType,
			ResourceID:    resourceID,
			UsageKind:     usageKind,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "media_asset_id"},
				{Name: "resource_type"},
				{Name: "resource_id"},
			},
			DoNothing: true,
		}).Create(&usage).Error; err != nil {
			return err
		}
	}
	return nil
}

func collectAssetIDs(values ...string) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	assetIDs := make([]uuid.UUID, 0)
	for _, value := range values {
		for _, match := range objectRefPattern.FindAllStringSubmatch(value, -1) {
			if len(match) != 2 || !strings.HasPrefix(match[0], objectRefPrefix) {
				continue
			}
			assetID, err := uuid.Parse(match[1])
			if err != nil {
				continue
			}
			if _, ok := seen[assetID]; ok {
				continue
			}
			seen[assetID] = struct{}{}
			assetIDs = append(assetIDs, assetID)
		}
	}
	return assetIDs
}

func projectWorkspaceID(project models.Project) uuid.UUID {
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return *project.WorkspaceID
	}
	return models.PersonalWorkspaceID(project.UserID)
}
