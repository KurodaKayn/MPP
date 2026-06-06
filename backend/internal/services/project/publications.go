package project

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *Service) GetProjectPublications(projectID uuid.UUID, scopeUserID *uuid.UUID, includeContent bool) (*dto.ProjectPublicationsResponse, error) {
	// Verify project exists and access
	var proj models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").Where("id = ?", projectID).First(&proj).Error; err != nil {
		return nil, err
	}

	if scopeUserID != nil {
		if _, err := s.ProjectAccessRole(proj, *scopeUserID); err != nil {
			return nil, err
		}
	}

	var publications []models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ?", projectID).Find(&publications).Error; err != nil {
		return nil, err
	}

	var items []dto.PublicationDetail
	for _, pub := range publications {
		// Safe parse config
		var rawConfig map[string]any
		_ = json.Unmarshal(pub.Config, &rawConfig)
		safeConfig := filterConfig(rawConfig)

		// Safe parse adapted content
		var rawContent map[string]any
		_ = json.Unmarshal(pub.AdaptedContent, &rawContent)
		safeContent := rawContent
		if !includeContent {
			safeContent = summarizeAdaptedContent(rawContent)
		}

		items = append(items, dto.PublicationDetail{
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
		})
	}

	if items == nil {
		items = []dto.PublicationDetail{}
	}

	return &dto.ProjectPublicationsResponse{
		ProjectID: projectID,
		Items:     items,
	}, nil
}

// Helper functions to filter sensitive data from JSONB fields

func filterConfig(raw map[string]any) map[string]any {
	safe := make(map[string]any)
	allowedKeys := []string{"title", "tags", "cover_image", "topics", "category", "original_declaration", "username"}
	for _, key := range allowedKeys {
		if val, ok := raw[key]; ok {
			safe[key] = val
		}
	}
	return safe
}

func summarizeAdaptedContent(raw map[string]any) map[string]any {
	safe := make(map[string]any)
	if summary, ok := raw["summary"]; ok {
		safe["summary"] = summary
	} else {
		safe["summary"] = "Content adapted (no summary available)"
	}
	if format, ok := raw["format"]; ok {
		safe["format"] = format
	}
	return safe
}
