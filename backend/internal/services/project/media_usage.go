package project

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	projectmediausage "github.com/kurodakayn/mpp-backend/internal/services/project/mediausage"
)

func refreshProjectMediaUsages(tx *gorm.DB, project models.Project, publications []models.ProjectPlatformPublication) error {
	return projectmediausage.RefreshProject(tx, project, publications)
}

func refreshContentTemplateMediaUsages(tx *gorm.DB, workspaceID uuid.UUID, template models.ContentTemplate) error {
	return projectmediausage.RefreshContentTemplate(tx, workspaceID, template)
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
