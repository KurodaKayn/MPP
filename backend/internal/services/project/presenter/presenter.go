package presenter

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

func ProjectDetailFromModel(project models.Project, role string, accessSource string) *dto.ProjectDetail {
	publications := PublicationSummariesFromModels(project.Publications)
	workspaceID := projectWorkspaceID(project)

	return &dto.ProjectDetail{
		ID:               project.ID,
		UserID:           project.UserID,
		WorkspaceID:      &workspaceID,
		CollabDocumentID: project.CollabDocumentID,
		TemplateID:       project.TemplateID,
		BrandProfileID:   project.BrandProfileID,
		Title:            project.Title,
		SourceContent:    project.SourceContent,
		Status:           project.Status,
		Role:             role,
		AccessSource:     accessSource,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     publications,
	}
}

func ProjectListItemFromModel(project models.Project, role string, accessSource string) dto.ProjectListItem {
	workspaceID := projectWorkspaceID(project)

	return dto.ProjectListItem{
		ID:               project.ID,
		UserID:           project.UserID,
		WorkspaceID:      &workspaceID,
		CollabDocumentID: project.CollabDocumentID,
		TemplateID:       project.TemplateID,
		BrandProfileID:   project.BrandProfileID,
		Title:            project.Title,
		Status:           project.Status,
		Role:             role,
		AccessSource:     accessSource,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     PublicationSummariesFromModels(project.Publications),
	}
}

func ProjectListItemFromSummary(summary models.ProjectListSummary, role string, accessSource string) (dto.ProjectListItem, error) {
	publications := []dto.PublicationSummary{}
	if len(summary.Publications) > 0 {
		if err := json.Unmarshal(summary.Publications, &publications); err != nil {
			return dto.ProjectListItem{}, err
		}
	}
	workspaceID := summary.WorkspaceID
	return dto.ProjectListItem{
		ID:               summary.ProjectID,
		UserID:           summary.UserID,
		WorkspaceID:      &workspaceID,
		CollabDocumentID: summary.CollabDocumentID,
		TemplateID:       summary.TemplateID,
		BrandProfileID:   summary.BrandProfileID,
		Title:            summary.Title,
		Status:           summary.Status,
		Role:             role,
		AccessSource:     accessSource,
		CreatedAt:        summary.CreatedAt,
		UpdatedAt:        summary.UpdatedAt,
		Publications:     publications,
	}, nil
}

func PublicationSummariesFromModels(publications []models.ProjectPlatformPublication) []dto.PublicationSummary {
	summaries := make([]dto.PublicationSummary, 0, len(publications))
	for _, pub := range publications {
		summaries = append(summaries, dto.PublicationSummary{
			ID:           pub.ID,
			Platform:     pub.Platform,
			Enabled:      pub.Enabled,
			Status:       pub.Status,
			DraftStatus:  pub.DraftStatus,
			ReviewStatus: pub.ReviewStatus,
			SyncRequired: pub.SyncRequired,
			PublishURL:   pub.PublishURL,
		})
	}
	if summaries == nil {
		return []dto.PublicationSummary{}
	}
	return summaries
}

func projectWorkspaceID(project models.Project) uuid.UUID {
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return *project.WorkspaceID
	}
	return models.PersonalWorkspaceID(project.UserID)
}
