package project

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *Service) enrichProjectDetail(detail *dto.ProjectDetail, project models.Project, userID *uuid.UUID) error {
	if detail == nil {
		return nil
	}
	detail.PublicationDetails = make([]dto.PublicationDetail, 0, len(project.Publications))
	for _, publication := range project.Publications {
		detail.PublicationDetails = append(detail.PublicationDetails, publicationDetailFromModel(publication, true))
	}
	if detail.PermissionSources == nil {
		detail.PermissionSources = []dto.ProjectPermissionSource{{
			Source: detail.AccessSource,
			Role:   detail.Role,
		}}
	}
	if userID == nil {
		return nil
	}

	comments, err := s.ListProjectComments(project.ID, *userID)
	if err != nil {
		return err
	}
	detail.Comments = comments.Items

	versions, err := s.ListProjectVersions(project.ID, *userID)
	if err != nil {
		return err
	}
	detail.Versions = versions.Items

	activities, err := s.ListProjectActivities(project.ID, *userID, 50)
	if err != nil {
		return err
	}
	detail.Activities = activities.Items

	if detail.Role == models.ProjectRoleOwner {
		collaborators, err := s.ListProjectCollaborators(project.ID, *userID)
		if err != nil {
			return err
		}
		detail.Collaborators = collaborators.Items
		links, err := s.ListProjectShareLinks(project.ID, *userID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if links != nil {
			detail.ShareLinks = links.Items
		}
		for _, collaborator := range detail.Collaborators {
			detail.PermissionSources = append(detail.PermissionSources, dto.ProjectPermissionSource{
				Source: models.ProjectAccessSourceDirectShare,
				Role:   collaborator.Role,
			})
		}
		for _, link := range detail.ShareLinks {
			if link.Status != models.ProjectShareLinkStatusActive {
				continue
			}
			detail.PermissionSources = append(detail.PermissionSources, dto.ProjectPermissionSource{
				Source: "share_link",
				Role:   link.Role,
			})
		}
	}

	return nil
}

func publicationDetailFromModel(pub models.ProjectPlatformPublication, includeContent bool) dto.PublicationDetail {
	var rawConfig map[string]any
	_ = json.Unmarshal(pub.Config, &rawConfig)
	safeConfig := filterConfig(rawConfig)

	var rawContent map[string]any
	_ = json.Unmarshal(pub.AdaptedContent, &rawContent)
	safeContent := rawContent
	if !includeContent {
		safeContent = summarizeAdaptedContent(rawContent)
	}
	if safeContent == nil {
		safeContent = map[string]any{}
	}

	return dto.PublicationDetail{
		ID:             pub.ID,
		Platform:       pub.Platform,
		Enabled:        pub.Enabled,
		Status:         pub.Status,
		DraftStatus:    pub.DraftStatus,
		ReviewStatus:   pub.ReviewStatus,
		SyncRequired:   pub.SyncRequired,
		ErrorMessage:   pub.ErrorMessage,
		Config:         safeConfig,
		AdaptedContent: safeContent,
		PublishURL:     pub.PublishURL,
		RemoteID:       pub.RemoteID,
		RetryCount:     pub.RetryCount,
		LastAttemptAt:  pub.LastAttemptAt,
		PublishedAt:    pub.PublishedAt,
		CreatedAt:      pub.CreatedAt,
		UpdatedAt:      pub.UpdatedAt,
	}
}
