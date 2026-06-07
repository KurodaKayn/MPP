package project

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

const mediaObjectRefPrefix = "mpp://media/"

var mediaObjectRefPattern = regexp.MustCompile(`mpp://media/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

func refreshProjectMediaUsages(tx *gorm.DB, project models.Project, publications []models.ProjectPlatformPublication) error {
	workspaceID := projectWorkspaceID(project)
	if err := tx.Where("project_id = ?", project.ID).Delete(&models.MediaAssetUsage{}).Error; err != nil {
		return err
	}

	sourceRefs := collectMediaAssetIDs(project.SourceContent)
	if err := upsertMediaUsages(tx, workspaceID, &project.ID, nil, nil, "project", project.ID, models.MediaAssetUsageEditorImage, sourceRefs); err != nil {
		return err
	}
	for _, publication := range publications {
		refs := collectMediaAssetIDs(string(publication.Config), string(publication.AdaptedContent))
		if len(refs) == 0 {
			continue
		}
		if err := upsertMediaUsages(tx, workspaceID, &project.ID, &publication.ID, nil, "publication", publication.ID, publication.Platform, refs); err != nil {
			return err
		}
	}
	return nil
}

func refreshContentTemplateMediaUsages(tx *gorm.DB, workspaceID uuid.UUID, template models.ContentTemplate) error {
	if template.ID == uuid.Nil {
		return ErrInvalidProject
	}
	if err := tx.Where("template_id = ?", template.ID).Delete(&models.MediaAssetUsage{}).Error; err != nil {
		return err
	}

	refs := collectMediaAssetIDs(template.SourceTemplate, string(template.PlatformConfig))
	return upsertMediaUsages(tx, workspaceID, nil, nil, &template.ID, "template", template.ID, models.MediaAssetUsageEditorImage, refs)
}

func (s *Service) RefreshProjectMediaUsages(projectID uuid.UUID) error {
	if projectID == uuid.Nil {
		return ErrInvalidProject
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		var publications []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", projectID).Find(&publications).Error; err != nil {
			return err
		}
		return refreshProjectMediaUsages(tx, project, publications)
	})
}

func upsertMediaUsages(tx *gorm.DB, workspaceID uuid.UUID, projectID *uuid.UUID, publicationID *uuid.UUID, templateID *uuid.UUID, resourceType string, resourceID uuid.UUID, usageKind string, assetIDs []uuid.UUID) error {
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

func collectMediaAssetIDs(values ...string) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	assetIDs := make([]uuid.UUID, 0)
	for _, value := range values {
		for _, match := range mediaObjectRefPattern.FindAllStringSubmatch(value, -1) {
			if len(match) != 2 || !strings.HasPrefix(match[0], mediaObjectRefPrefix) {
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
