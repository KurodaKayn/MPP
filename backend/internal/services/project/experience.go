package project

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	projectexperience "github.com/kurodakayn/mpp-backend/internal/services/project/experience"
)

var ErrInvalidProjectComment = projectexperience.ErrInvalidProjectComment
var ErrInvalidProjectShareLink = projectexperience.ErrInvalidProjectShareLink
var ErrInvalidProjectVersion = projectexperience.ErrInvalidProjectVersion

func (s *Service) experienceService() *projectexperience.Service {
	return projectexperience.NewService(
		s.db,
		s.GetProject,
		refreshProjectMediaUsages,
		s.invalidateDashboardCaches,
		s.invalidateDashboardScopedStatsCache,
	)
}

func (s *Service) ListProjectActivities(projectID uuid.UUID, userID uuid.UUID, limit int) (*dto.ProjectActivitiesResponse, error) {
	return s.experienceService().ListProjectActivities(projectID, userID, limit)
}

func (s *Service) ListProjectComments(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectCommentsResponse, error) {
	return s.experienceService().ListProjectComments(projectID, userID)
}

func (s *Service) CreateProjectComment(projectID uuid.UUID, userID uuid.UUID, req dto.CreateProjectCommentRequest) (*dto.ProjectComment, error) {
	return s.experienceService().CreateProjectComment(projectID, userID, req)
}

func (s *Service) UpdateProjectComment(projectID uuid.UUID, userID uuid.UUID, commentID uuid.UUID, req dto.UpdateProjectCommentRequest) (*dto.ProjectComment, error) {
	return s.experienceService().UpdateProjectComment(projectID, userID, commentID, req)
}

func (s *Service) ListProjectVersions(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectVersionsResponse, error) {
	return s.experienceService().ListProjectVersions(projectID, userID)
}

func (s *Service) RestoreProjectVersion(projectID uuid.UUID, userID uuid.UUID, versionID uuid.UUID) (*dto.RestoreProjectVersionResponse, error) {
	return s.experienceService().RestoreProjectVersion(projectID, userID, versionID)
}

func (s *Service) ListProjectShareLinks(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectShareLinksResponse, error) {
	return s.experienceService().ListProjectShareLinks(projectID, userID)
}

func (s *Service) CreateProjectShareLink(projectID uuid.UUID, userID uuid.UUID, req dto.CreateProjectShareLinkRequest, baseURL string) (*dto.ProjectShareLinkWithToken, error) {
	return s.experienceService().CreateProjectShareLink(projectID, userID, req, baseURL)
}

func (s *Service) AcceptProjectShareLink(token string, userID uuid.UUID) (*dto.AcceptProjectShareLinkResponse, error) {
	return s.experienceService().AcceptProjectShareLink(token, userID)
}

func (s *Service) RevokeProjectShareLink(projectID uuid.UUID, userID uuid.UUID, linkID uuid.UUID) error {
	return s.experienceService().RevokeProjectShareLink(projectID, userID, linkID)
}

func recordProjectActivity(tx *gorm.DB, projectID uuid.UUID, actorUserID uuid.UUID, targetUserID *uuid.UUID, eventType string, metadata map[string]any) error {
	return projectexperience.RecordProjectActivity(tx, projectID, actorUserID, targetUserID, eventType, metadata)
}

func createProjectVersion(tx *gorm.DB, project models.Project, userID uuid.UUID, source string) error {
	return projectexperience.CreateProjectVersion(tx, project, userID, source)
}
