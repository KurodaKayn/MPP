package project

import (
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/project/contentsetup"
)

func (s *Service) contentSetupService() *contentsetup.Service {
	return contentsetup.NewService(s.db, contentsetup.CacheConfig{
		Client: s.cache,
		TTL:    s.cacheTTL,
		Group:  s.cacheGroup,
		Guard:  s.contentSetupGuard,
	}, contentsetup.Dependencies{
		NormalizeProjectPlatforms:       NormalizeProjectPlatforms,
		SanitizeProjectSourceContent:    sanitizeProjectSourceContent,
		EnsurePersonalWorkspace:         ensurePersonalWorkspace,
		RefreshContentTemplateMediaUses: refreshContentTemplateMediaUsages,
	})
}

func (s *Service) ListContentTemplates(userID uuid.UUID, workspaceID uuid.UUID) (*dto.ContentTemplatesResponse, error) {
	return s.contentSetupService().ListContentTemplates(userID, workspaceID)
}

func (s *Service) CreateContentTemplate(userID uuid.UUID, workspaceID uuid.UUID, req dto.CreateContentTemplateRequest) (*dto.ContentTemplate, error) {
	return s.contentSetupService().CreateContentTemplate(userID, workspaceID, req)
}

func (s *Service) ListBrandProfiles(userID uuid.UUID, workspaceID uuid.UUID) (*dto.BrandProfilesResponse, error) {
	return s.contentSetupService().ListBrandProfiles(userID, workspaceID)
}

func (s *Service) CreateBrandProfile(userID uuid.UUID, workspaceID uuid.UUID, req dto.CreateBrandProfileRequest) (*dto.BrandProfile, error) {
	return s.contentSetupService().CreateBrandProfile(userID, workspaceID, req)
}

func (s *Service) contentTemplateForProject(userID uuid.UUID, workspaceID uuid.UUID, templateID *uuid.UUID) (models.ContentTemplate, bool, error) {
	return s.contentSetupService().ContentTemplateForProject(userID, workspaceID, templateID)
}

func (s *Service) validateBrandProfileForProject(userID uuid.UUID, workspaceID uuid.UUID, brandProfileID *uuid.UUID) error {
	return s.contentSetupService().ValidateBrandProfileForProject(userID, workspaceID, brandProfileID)
}

func contentTemplateDefaultPlatforms(template *models.ContentTemplate) ([]string, error) {
	return contentsetup.ContentTemplateDefaultPlatforms(template, NormalizeProjectPlatforms)
}

func contentTemplatePlatformConfig(template *models.ContentTemplate, platform string) map[string]any {
	return contentsetup.ContentTemplatePlatformConfig(template, platform)
}

func mergePublicationConfig(base datatypes.JSON, extra map[string]any) (datatypes.JSON, error) {
	return contentsetup.MergePublicationConfig(base, extra)
}
