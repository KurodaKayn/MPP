package contentsetup

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	"github.com/kurodakayn/mpp-backend/internal/services/project/projecterr"
)

type NormalizeProjectPlatformsFunc func([]string) ([]string, error)
type SanitizeProjectSourceContentFunc func(string) string
type EnsurePersonalWorkspaceFunc func(*gorm.DB, uuid.UUID) error
type ContentTemplateMediaUsageRefresher func(*gorm.DB, uuid.UUID, models.ContentTemplate) error

type Dependencies struct {
	NormalizeProjectPlatforms       NormalizeProjectPlatformsFunc
	SanitizeProjectSourceContent    SanitizeProjectSourceContentFunc
	EnsurePersonalWorkspace         EnsurePersonalWorkspaceFunc
	RefreshContentTemplateMediaUses ContentTemplateMediaUsageRefresher
}

type CacheConfig struct {
	Client redis.UniversalClient
	TTL    time.Duration
	Group  *singleflight.Group
	Guard  *redisdegrade.Guard
}

type Service struct {
	db                *gorm.DB
	cache             redis.UniversalClient
	cacheTTL          time.Duration
	cacheGroup        *singleflight.Group
	contentSetupGuard *redisdegrade.Guard
	deps              Dependencies
}

func NewService(db *gorm.DB, cache CacheConfig, deps Dependencies) *Service {
	return &Service{
		db:                db,
		cache:             cache.Client,
		cacheTTL:          cache.TTL,
		cacheGroup:        cache.Group,
		contentSetupGuard: cache.Guard,
		deps:              deps,
	}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	scoped.cacheGroup = s.cacheGroup
	return &scoped
}

func (s *Service) ListContentTemplates(userID uuid.UUID, workspaceID uuid.UUID) (*dto.ContentTemplatesResponse, error) {
	if userID == uuid.Nil {
		return nil, projecterr.ErrInvalidProject
	}
	workspaceID = workspaceIDForUser(userID, workspaceID)
	if err := s.ensurePersonalWorkspaceForUser(userID, workspaceID); err != nil {
		return nil, err
	}
	if workspaceID != models.PersonalWorkspaceID(userID) {
		if _, err := accesspolicy.WorkspaceProjectRoleWithDB(s.db, workspaceID, userID); err != nil {
			return nil, err
		}
	}
	return s.getCachedContentTemplates(userID, workspaceID)
}

func (s *Service) computeContentTemplates(userID uuid.UUID, workspaceID uuid.UUID) (*dto.ContentTemplatesResponse, error) {
	var templates []models.ContentTemplate
	if err := s.db.
		Where("scope = ?", models.ContentTemplateScopeSystem).
		Or("scope = ? AND owner_user_id = ?", models.ContentTemplateScopePersonal, userID).
		Or("scope = ? AND workspace_id = ?", models.ContentTemplateScopeWorkspace, workspaceID).
		Order("scope asc, updated_at desc, name asc").
		Find(&templates).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ContentTemplate, 0, len(templates))
	for _, template := range templates {
		items = append(items, contentTemplateFromModel(template))
	}
	return &dto.ContentTemplatesResponse{Items: items}, nil
}

func (s *Service) CreateContentTemplate(userID uuid.UUID, workspaceID uuid.UUID, req dto.CreateContentTemplateRequest) (*dto.ContentTemplate, error) {
	if userID == uuid.Nil {
		return nil, projecterr.ErrInvalidProject
	}
	workspaceID = workspaceIDForUser(userID, workspaceID)
	if err := s.ensurePersonalWorkspaceForUser(userID, workspaceID); err != nil {
		return nil, err
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = models.ContentTemplateScopePersonal
		if workspaceID != models.PersonalWorkspaceID(userID) {
			scope = models.ContentTemplateScopeWorkspace
		}
	}
	if scope == models.ContentTemplateScopeSystem {
		return nil, accesspolicy.ErrForbidden
	}
	if scope != models.ContentTemplateScopePersonal && scope != models.ContentTemplateScopeWorkspace {
		return nil, projecterr.ErrInvalidProject
	}
	if scope == models.ContentTemplateScopeWorkspace {
		role, err := accesspolicy.WorkspaceProjectRoleWithDB(s.db, workspaceID, userID)
		if err != nil {
			return nil, err
		}
		if !accesspolicy.CanEditProjectRole(role) {
			return nil, accesspolicy.ErrForbidden
		}
	}

	name := strings.TrimSpace(req.Name)
	titleTemplate := strings.TrimSpace(req.TitleTemplate)
	sourceTemplate := s.deps.SanitizeProjectSourceContent(req.SourceTemplate)
	platforms, err := s.deps.NormalizeProjectPlatforms(req.DefaultPlatforms)
	if err != nil {
		return nil, err
	}
	if name == "" || titleTemplate == "" || sourceTemplate == "" || len(platforms) == 0 {
		return nil, projecterr.ErrInvalidProject
	}

	defaultPlatforms, err := json.Marshal(platforms)
	if err != nil {
		return nil, err
	}
	platformConfig, err := json.Marshal(req.PlatformConfig)
	if err != nil {
		return nil, err
	}
	tags, err := json.Marshal(normalizeStringList(req.Tags))
	if err != nil {
		return nil, err
	}

	template := models.ContentTemplate{
		Scope:            scope,
		Name:             name,
		Description:      strings.TrimSpace(req.Description),
		TitleTemplate:    titleTemplate,
		SourceTemplate:   sourceTemplate,
		DefaultPlatforms: datatypes.JSON(defaultPlatforms),
		PlatformConfig:   datatypes.JSON(platformConfig),
		Tags:             datatypes.JSON(tags),
	}
	if scope == models.ContentTemplateScopeWorkspace {
		template.WorkspaceID = &workspaceID
	} else {
		ownerID := userID
		template.OwnerUserID = &ownerID
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&template).Error; err != nil {
			return err
		}
		return s.deps.RefreshContentTemplateMediaUses(tx, workspaceID, template)
	}); err != nil {
		return nil, err
	}
	s.InvalidateContentTemplateOptionsCache(userID, workspaceID, scope)
	resp := contentTemplateFromModel(template)
	return &resp, nil
}

func (s *Service) ListBrandProfiles(userID uuid.UUID, workspaceID uuid.UUID) (*dto.BrandProfilesResponse, error) {
	if userID == uuid.Nil {
		return nil, projecterr.ErrInvalidProject
	}
	workspaceID = workspaceIDForUser(userID, workspaceID)
	if err := s.ensurePersonalWorkspaceForUser(userID, workspaceID); err != nil {
		return nil, err
	}
	if _, err := accesspolicy.WorkspaceProjectRoleWithDB(s.db, workspaceID, userID); err != nil {
		return nil, err
	}
	return s.getCachedBrandProfiles(userID, workspaceID)
}

func (s *Service) computeBrandProfiles(workspaceID uuid.UUID) (*dto.BrandProfilesResponse, error) {
	var profiles []models.BrandProfile
	if err := s.db.Where("workspace_id = ?", workspaceID).Order("updated_at desc, name asc").Find(&profiles).Error; err != nil {
		return nil, err
	}
	items := make([]dto.BrandProfile, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, brandProfileFromModel(profile))
	}
	return &dto.BrandProfilesResponse{Items: items}, nil
}

func (s *Service) CreateBrandProfile(userID uuid.UUID, workspaceID uuid.UUID, req dto.CreateBrandProfileRequest) (*dto.BrandProfile, error) {
	if userID == uuid.Nil {
		return nil, projecterr.ErrInvalidProject
	}
	workspaceID = workspaceIDForUser(userID, workspaceID)
	if err := s.ensurePersonalWorkspaceForUser(userID, workspaceID); err != nil {
		return nil, err
	}
	role, err := accesspolicy.WorkspaceProjectRoleWithDB(s.db, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	if !accesspolicy.CanEditProjectRole(role) {
		return nil, accesspolicy.ErrForbidden
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, projecterr.ErrInvalidProject
	}
	bannedWords, err := json.Marshal(normalizeStringList(req.BannedWords))
	if err != nil {
		return nil, err
	}
	defaultTags, err := json.Marshal(normalizeStringList(req.DefaultTags))
	if err != nil {
		return nil, err
	}
	profile := models.BrandProfile{
		WorkspaceID:  workspaceID,
		CreatedBy:    userID,
		Name:         name,
		Voice:        strings.TrimSpace(req.Voice),
		Audience:     strings.TrimSpace(req.Audience),
		BannedWords:  datatypes.JSON(bannedWords),
		CTA:          strings.TrimSpace(req.CTA),
		LinkStrategy: strings.TrimSpace(req.LinkStrategy),
		DefaultTags:  datatypes.JSON(defaultTags),
	}
	if err := s.db.Create(&profile).Error; err != nil {
		return nil, err
	}
	s.InvalidateBrandProfileOptionsCache(workspaceID)
	resp := brandProfileFromModel(profile)
	return &resp, nil
}

func (s *Service) ContentTemplateForProject(userID uuid.UUID, workspaceID uuid.UUID, templateID *uuid.UUID) (models.ContentTemplate, bool, error) {
	if templateID == nil || *templateID == uuid.Nil {
		return models.ContentTemplate{}, false, nil
	}
	var template models.ContentTemplate
	if err := s.db.First(&template, "id = ?", *templateID).Error; err != nil {
		return models.ContentTemplate{}, false, err
	}
	if contentTemplateAccessible(template, userID, workspaceID) {
		return template, true, nil
	}
	return models.ContentTemplate{}, false, accesspolicy.ErrForbidden
}

func (s *Service) ValidateBrandProfileForProject(userID uuid.UUID, workspaceID uuid.UUID, brandProfileID *uuid.UUID) error {
	if brandProfileID == nil || *brandProfileID == uuid.Nil {
		return nil
	}
	var profile models.BrandProfile
	if err := s.db.First(&profile, "id = ?", *brandProfileID).Error; err != nil {
		return err
	}
	if profile.WorkspaceID != workspaceID {
		return accesspolicy.ErrForbidden
	}
	if _, err := accesspolicy.WorkspaceProjectRoleWithDB(s.db, workspaceID, userID); err != nil {
		return err
	}
	return nil
}

func contentTemplateAccessible(template models.ContentTemplate, userID uuid.UUID, workspaceID uuid.UUID) bool {
	switch template.Scope {
	case models.ContentTemplateScopeSystem:
		return true
	case models.ContentTemplateScopeWorkspace:
		return template.WorkspaceID != nil && *template.WorkspaceID == workspaceID
	case models.ContentTemplateScopePersonal:
		return template.OwnerUserID != nil && *template.OwnerUserID == userID
	default:
		return false
	}
}

func contentTemplateFromModel(template models.ContentTemplate) dto.ContentTemplate {
	return dto.ContentTemplate{
		ID:               template.ID,
		WorkspaceID:      template.WorkspaceID,
		OwnerUserID:      template.OwnerUserID,
		Scope:            template.Scope,
		Name:             template.Name,
		Description:      template.Description,
		TitleTemplate:    template.TitleTemplate,
		SourceTemplate:   template.SourceTemplate,
		DefaultPlatforms: stringListFromJSON(template.DefaultPlatforms),
		PlatformConfig:   mapFromJSON(template.PlatformConfig),
		Tags:             stringListFromJSON(template.Tags),
		CreatedAt:        template.CreatedAt,
		UpdatedAt:        template.UpdatedAt,
	}
}

func brandProfileFromModel(profile models.BrandProfile) dto.BrandProfile {
	return dto.BrandProfile{
		ID:           profile.ID,
		WorkspaceID:  profile.WorkspaceID,
		CreatedBy:    profile.CreatedBy,
		Name:         profile.Name,
		Voice:        profile.Voice,
		Audience:     profile.Audience,
		BannedWords:  stringListFromJSON(profile.BannedWords),
		CTA:          profile.CTA,
		LinkStrategy: profile.LinkStrategy,
		DefaultTags:  stringListFromJSON(profile.DefaultTags),
		CreatedAt:    profile.CreatedAt,
		UpdatedAt:    profile.UpdatedAt,
	}
}

func workspaceIDForUser(userID uuid.UUID, workspaceID uuid.UUID) uuid.UUID {
	if workspaceID != uuid.Nil {
		return workspaceID
	}
	return models.PersonalWorkspaceID(userID)
}

func (s *Service) ensurePersonalWorkspaceForUser(userID uuid.UUID, workspaceID uuid.UUID) error {
	if workspaceID != models.PersonalWorkspaceID(userID) {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.deps.EnsurePersonalWorkspace(tx, userID)
	})
}

func normalizeStringList(values []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func stringListFromJSON(raw datatypes.JSON) []string {
	items := []string{}
	_ = json.Unmarshal(raw, &items)
	return items
}

func mapFromJSON(value datatypes.JSON) map[string]any {
	var parsed map[string]any
	if err := json.Unmarshal(value, &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func ContentTemplateDefaultPlatforms(template *models.ContentTemplate, normalize NormalizeProjectPlatformsFunc) ([]string, error) {
	if template == nil {
		return nil, nil
	}
	platforms := stringListFromJSON(template.DefaultPlatforms)
	return normalize(platforms)
}

func ContentTemplatePlatformConfig(template *models.ContentTemplate, platform string) map[string]any {
	if template == nil {
		return nil
	}
	config := mapFromJSON(template.PlatformConfig)
	if platformConfig, ok := config[platform].(map[string]any); ok {
		return platformConfig
	}
	return nil
}

func MergePublicationConfig(base datatypes.JSON, extra map[string]any) (datatypes.JSON, error) {
	if len(extra) == 0 {
		return base, nil
	}
	config := mapFromJSON(base)
	maps.Copy(config, extra)
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}
