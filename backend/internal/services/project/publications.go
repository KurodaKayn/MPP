package project

import (
	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	projectpublication "github.com/kurodakayn/mpp-backend/internal/services/project/publication"
	"github.com/kurodakayn/mpp-backend/internal/services/project/publicationselection"
)

func NormalizeProjectPlatforms(input []string) ([]string, error) {
	return projectpublication.NormalizePlatforms(input)
}

func pendingPublicationConfigForTemplate(title, summary, coverImageURL string, template *models.ContentTemplate) publicationselection.ConfigForPlatform {
	return projectpublication.PendingConfigForTemplate(title, summary, coverImageURL, template)
}

func defaultPublicationConfigForProjectTitle(title string) publicationselection.ConfigForPlatform {
	return projectpublication.DefaultConfigForProjectTitle(title)
}

func (s *Service) GetProjectPublications(projectID uuid.UUID, scopeUserID *uuid.UUID, includeContent bool) (*dto.ProjectPublicationsResponse, error) {
	readDB := s.projectDetailReadDB(scopeUserID)

	// Verify project exists and access
	var proj models.Project
	if err := readDB.Select("id", "user_id", "workspace_id").Where("id = ?", projectID).First(&proj).Error; err != nil {
		return nil, err
	}

	if scopeUserID != nil {
		if _, err := s.ProjectAccessRole(proj, *scopeUserID); err != nil {
			return nil, err
		}
	}

	var publications []models.ProjectPlatformPublication
	if err := readDB.Where("project_id = ?", projectID).Find(&publications).Error; err != nil {
		return nil, err
	}

	var items []dto.PublicationDetail
	for _, pub := range publications {
		items = append(items, projectpublication.ResponseDetailFromModel(pub, includeContent))
	}

	if items == nil {
		items = []dto.PublicationDetail{}
	}

	return &dto.ProjectPublicationsResponse{
		ProjectID: projectID,
		Items:     items,
	}, nil
}
