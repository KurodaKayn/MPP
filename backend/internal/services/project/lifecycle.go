package project

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkghtml "github.com/kurodakayn/mpp-backend/internal/pkg/html"
)

func (s *Service) CreateProject(userID uuid.UUID, req dto.CreateProjectRequest) (*dto.ProjectListItem, error) {
	workspaceID := models.PersonalWorkspaceID(userID)
	return s.CreateProjectWithWorkspace(userID, &workspaceID, req)
}

func (s *Service) CreateProjectWithWorkspace(userID uuid.UUID, workspaceID *uuid.UUID, req dto.CreateProjectRequest) (*dto.ProjectListItem, error) {
	workspaceValue := models.PersonalWorkspaceID(userID)
	if workspaceID != nil && *workspaceID != uuid.Nil {
		workspaceValue = *workspaceID
	}
	templateValue, hasTemplate, err := s.contentTemplateForProject(userID, workspaceValue, req.TemplateID)
	if err != nil {
		return nil, err
	}
	var template *models.ContentTemplate
	if hasTemplate {
		template = &templateValue
	}
	if err := s.validateBrandProfileForProject(userID, workspaceValue, req.BrandProfileID); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" && template != nil {
		title = template.TitleTemplate
	}
	sourceInput := req.SourceContent
	if strings.TrimSpace(sourceInput) == "" && template != nil {
		sourceInput = template.SourceTemplate
	}
	sourceContent := sanitizeProjectSourceContent(sourceInput)
	platformInput := req.Platforms
	if len(platformInput) == 0 {
		platformInput, err = contentTemplateDefaultPlatforms(template)
		if err != nil {
			return nil, err
		}
	}
	platforms, err := NormalizeProjectPlatforms(platformInput)
	if err != nil {
		return nil, err
	}
	if title == "" || sourceContent == "" || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	project := models.Project{
		UserID:         userID,
		WorkspaceID:    workspaceID,
		TemplateID:     req.TemplateID,
		BrandProfileID: req.BrandProfileID,
		Title:          title,
		SourceContent:  sourceContent,
		Status:         models.ProjectStatusReady,
	}
	publications := make([]dto.PublicationSummary, 0, len(platforms))
	createdPublications := make([]models.ProjectPlatformPublication, 0, len(platforms))

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if workspaceID != nil && *workspaceID == models.PersonalWorkspaceID(userID) {
			if err := ensurePersonalWorkspace(tx, userID); err != nil {
				return err
			}
		}

		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		for _, platform := range platforms {
			config, adaptedContent, status, err := BuildPendingPublicationPayload(title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			if config, err = mergePublicationConfig(config, contentTemplatePlatformConfig(template, platform)); err != nil {
				return err
			}

			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				SyncRequired:   true,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}
			createdPublications = append(createdPublications, publication)

			publications = append(publications, dto.PublicationSummary{
				ID:           publication.ID,
				Platform:     platform,
				Enabled:      publication.Enabled,
				Status:       publication.Status,
				DraftStatus:  publication.DraftStatus,
				ReviewStatus: publication.ReviewStatus,
				SyncRequired: publication.SyncRequired,
			})
		}

		if err := refreshProjectMediaUsages(tx, project, createdPublications); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.invalidateDashboardCaches(true)
	s.refreshProjectReadModel(project.ID)

	return &dto.ProjectListItem{
		ID:               project.ID,
		UserID:           project.UserID,
		WorkspaceID:      project.WorkspaceID,
		CollabDocumentID: project.CollabDocumentID,
		TemplateID:       project.TemplateID,
		BrandProfileID:   project.BrandProfileID,
		Title:            project.Title,
		Status:           project.Status,
		Role:             models.ProjectRoleOwner,
		AccessSource:     models.ProjectAccessSourceOwner,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     publications,
	}, nil
}

func (s *Service) GetProject(projectID uuid.UUID, scopeUserID *uuid.UUID) (*dto.ProjectDetail, error) {
	var project models.Project
	if err := s.projectDetailReadDB(scopeUserID).
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, draft_status, review_status, sync_required, publish_url").Order("platform asc")
		}).
		First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}

	role := models.ProjectRoleOwner
	accessSource := models.ProjectAccessSourceOwner
	if scopeUserID != nil {
		accessRole, source, err := s.ProjectAccessRoleAndSource(project, *scopeUserID)
		if err != nil {
			return nil, err
		}
		role = accessRole
		accessSource = source
	}

	detail := projectDetailFromModel(project, role, accessSource)
	if err := s.enrichProjectDetail(detail, project, scopeUserID); err != nil {
		return nil, err
	}
	return detail, nil
}

func (s *Service) projectDetailReadDB(scopeUserID *uuid.UUID) *gorm.DB {
	if scopeUserID != nil {
		return s.strongReadDB()
	}
	return s.eventualReadDB()
}

func (s *Service) UpdateProject(projectID uuid.UUID, userID uuid.UUID, req dto.UpdateProjectRequest) (*dto.ProjectDetail, error) {
	var existingProject models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&existingProject, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.ProjectAccessRole(existingProject, userID)
	if err != nil {
		return nil, err
	}
	if !CanEditProjectRole(role) {
		return nil, ErrForbidden
	}
	workspaceValue := projectWorkspaceID(existingProject)
	templateValue, hasTemplate, err := s.contentTemplateForProject(userID, workspaceValue, req.TemplateID)
	if err != nil {
		return nil, err
	}
	var template *models.ContentTemplate
	if hasTemplate {
		template = &templateValue
	}
	if err := s.validateBrandProfileForProject(userID, workspaceValue, req.BrandProfileID); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" && template != nil {
		title = template.TitleTemplate
	}
	sourceInput := req.SourceContent
	if strings.TrimSpace(sourceInput) == "" && template != nil {
		sourceInput = template.SourceTemplate
	}
	sourceContent := sanitizeProjectSourceContent(sourceInput)
	platformInput := req.Platforms
	if len(platformInput) == 0 {
		platformInput, err = contentTemplateDefaultPlatforms(template)
		if err != nil {
			return nil, err
		}
	}
	platforms, err := NormalizeProjectPlatforms(platformInput)
	if err != nil {
		return nil, err
	}
	if title == "" || sourceContent == "" || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	syncedCollabSource, err := s.SyncProjectCollabSourceContentIfMaterialized(projectID, userID)
	if err != nil {
		return nil, err
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}

		project.Title = title
		project.TemplateID = req.TemplateID
		project.BrandProfileID = req.BrandProfileID
		if project.CollabDocumentID == nil || *project.CollabDocumentID == uuid.Nil || !syncedCollabSource {
			project.SourceContent = sourceContent
		}
		project.SourceContent = sanitizeProjectSourceContent(project.SourceContent)
		if project.SourceContent == "" {
			return ErrInvalidProject
		}
		project.Status = models.ProjectStatusReady
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		if err := createProjectVersion(tx, project, userID, "project_update"); err != nil {
			return err
		}
		if err := recordProjectActivity(tx, project.ID, userID, nil, models.ProjectActivityContentSaved, map[string]any{
			"title": project.Title,
		}); err != nil {
			return err
		}

		var existing []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&existing).Error; err != nil {
			return err
		}

		selected := make(map[string]struct{}, len(platforms))
		for _, platform := range platforms {
			selected[platform] = struct{}{}
		}

		for _, publication := range existing {
			if _, ok := selected[publication.Platform]; !ok {
				if err := tx.Model(&publication).Updates(map[string]any{
					"enabled":       false,
					"error_message": "",
					"status":        models.PublicationStatusDisabled,
				}).Error; err != nil {
					return err
				}
				continue
			}

			config, err := defaultPublicationConfig(title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			if config, err = mergePublicationConfig(config, contentTemplatePlatformConfig(template, publication.Platform)); err != nil {
				return err
			}
			if err := tx.Model(&publication).Updates(map[string]any{
				"config":          config,
				"enabled":         true,
				"error_message":   "",
				"draft_status":    models.PublicationDraftStatusStale,
				"review_status":   models.PublicationReviewStatusDraft,
				"sync_required":   true,
				"last_attempt_at": nil,
				"published_at":    nil,
				"publish_url":     "",
				"remote_id":       "",
				"retry_count":     0,
				"status":          models.PublicationStatusPending,
			}).Error; err != nil {
				return err
			}
			delete(selected, publication.Platform)
		}

		for _, platform := range platforms {
			if _, ok := selected[platform]; !ok {
				continue
			}

			config, adaptedContent, status, err := BuildPendingPublicationPayload(title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			if config, err = mergePublicationConfig(config, contentTemplatePlatformConfig(template, platform)); err != nil {
				return err
			}
			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				SyncRequired:   true,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}
		}

		var publications []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&publications).Error; err != nil {
			return err
		}
		if err := refreshProjectMediaUsages(tx, project, publications); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.invalidateDashboardCaches(true)
	s.refreshProjectReadModel(projectID)

	return s.GetProject(projectID, &userID)
}

func (s *Service) SaveProjectContent(projectID uuid.UUID, userID uuid.UUID, req dto.SaveProjectContentRequest) (*dto.ProjectDetail, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := sanitizeProjectSourceContent(req.SourceContent)
	if title == "" || sourceContent == "" {
		return nil, ErrInvalidProject
	}

	var project models.Project
	if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.ProjectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !CanEditProjectRole(role) {
		return nil, ErrForbidden
	}

	syncedCollabSource, err := s.syncProjectSourceContentDocumentIfMaterialized(project.CollabDocumentID)
	if err != nil {
		return nil, err
	}
	if syncedCollabSource {
		if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
			return nil, err
		}
	}

	if project.CollabDocumentID == nil || *project.CollabDocumentID == uuid.Nil || !syncedCollabSource {
		project.SourceContent = sourceContent
	}
	project.SourceContent = sanitizeProjectSourceContent(project.SourceContent)
	if project.SourceContent == "" {
		return nil, ErrInvalidProject
	}
	project.Title = title
	project.Status = models.ProjectStatusReady

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		if err := createProjectVersion(tx, project, userID, "content_save"); err != nil {
			return err
		}
		if err := markProjectDraftsStale(tx, project.ID); err != nil {
			return err
		}
		var publications []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&publications).Error; err != nil {
			return err
		}
		if err := refreshProjectMediaUsages(tx, project, publications); err != nil {
			return err
		}
		return recordProjectActivity(tx, project.ID, userID, nil, models.ProjectActivityContentSaved, map[string]any{
			"title": project.Title,
		})
	}); err != nil {
		return nil, err
	}
	s.invalidateDashboardCaches(false)
	s.refreshProjectReadModel(projectID)

	return s.GetProject(projectID, &userID)
}

func (s *Service) SaveProjectPlatforms(projectID uuid.UUID, userID uuid.UUID, req dto.SaveProjectPlatformsRequest) (*dto.ProjectDetail, error) {
	platforms, err := NormalizeProjectPlatforms(req.Platforms)
	if err != nil || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		role, err := ProjectAccessRoleWithDB(tx, project, userID)
		if err != nil {
			return err
		}
		if !CanEditProjectRole(role) {
			return ErrForbidden
		}

		var existing []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&existing).Error; err != nil {
			return err
		}

		selected := make(map[string]struct{}, len(platforms))
		for _, platform := range platforms {
			selected[platform] = struct{}{}
		}

		for _, publication := range existing {
			if _, ok := selected[publication.Platform]; !ok {
				if err := tx.Model(&publication).Updates(map[string]any{
					"enabled":       false,
					"error_message": "",
					"status":        models.PublicationStatusDisabled,
				}).Error; err != nil {
					return err
				}
				continue
			}

			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				if err := tx.Model(&publication).Updates(map[string]any{
					"draft_status":  models.PublicationDraftStatusUnsynced,
					"enabled":       true,
					"review_status": models.PublicationReviewStatusDraft,
					"status":        models.PublicationStatusPending,
					"sync_required": true,
				}).Error; err != nil {
					return err
				}
			}
			delete(selected, publication.Platform)
		}

		for _, platform := range platforms {
			if _, ok := selected[platform]; !ok {
				continue
			}

			config, adaptedContent, status, err := BuildPendingPublicationPayload(project.Title, "", "")
			if err != nil {
				return err
			}
			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				SyncRequired:   true,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}
		}

		var publications []models.ProjectPlatformPublication
		if err := tx.Where("project_id = ?", project.ID).Find(&publications).Error; err != nil {
			return err
		}
		if err := refreshProjectMediaUsages(tx, project, publications); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.invalidateDashboardCaches(true)
	s.refreshProjectReadModel(projectID)

	return s.GetProject(projectID, &userID)
}

func BuildPendingPublicationPayload(title, summary, coverImageURL string) (datatypes.JSON, datatypes.JSON, string, error) {
	config, err := defaultPublicationConfig(title, summary, coverImageURL)
	if err != nil {
		return nil, nil, "", err
	}

	return config, datatypes.JSON([]byte(`{}`)), models.PublicationStatusPending, nil
}

func sanitizeProjectSourceContent(sourceContent string) string {
	return pkghtml.SanitizeStoredHTML(strings.TrimSpace(sourceContent))
}

func projectWorkspaceID(project models.Project) uuid.UUID {
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return *project.WorkspaceID
	}
	return models.PersonalWorkspaceID(project.UserID)
}

func markProjectDraftsStale(tx *gorm.DB, projectID uuid.UUID) error {
	return tx.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND enabled = ? AND status NOT IN ?", projectID, true, []string{
			models.PublicationStatusQueued,
			models.PublicationStatusPublishing,
			models.PublicationStatusSucceeded,
		}).
		Updates(map[string]any{
			"draft_status":  models.PublicationDraftStatusStale,
			"review_status": models.PublicationReviewStatusDraft,
			"sync_required": true,
		}).Error
}

func projectDetailFromModel(project models.Project, role string, accessSource string) *dto.ProjectDetail {
	publications := make([]dto.PublicationSummary, 0, len(project.Publications))
	for _, pub := range project.Publications {
		publications = append(publications, dto.PublicationSummary{
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
	if publications == nil {
		publications = []dto.PublicationSummary{}
	}
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

func projectListItemFromModel(project models.Project, access projectAccessResolution) dto.ProjectListItem {
	publications := make([]dto.PublicationSummary, 0, len(project.Publications))
	for _, pub := range project.Publications {
		publications = append(publications, dto.PublicationSummary{
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
	if publications == nil {
		publications = []dto.PublicationSummary{}
	}
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
		Role:             access.role,
		AccessSource:     access.source,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     publications,
	}
}

func projectListItemFromSummary(summary models.ProjectListSummary, access projectAccessResolution) (dto.ProjectListItem, error) {
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
		Role:             access.role,
		AccessSource:     access.source,
		CreatedAt:        summary.CreatedAt,
		UpdatedAt:        summary.UpdatedAt,
		Publications:     publications,
	}, nil
}

func NormalizeProjectPlatforms(input []string) ([]string, error) {
	seen := map[string]struct{}{}
	platforms := make([]string, 0, len(input))

	for _, raw := range input {
		platform := strings.TrimSpace(raw)
		if platform == "" {
			continue
		}
		if _, ok := allowedProjectPlatforms[platform]; !ok {
			return nil, ErrInvalidProject
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		platforms = append(platforms, platform)
	}

	return platforms, nil
}

func defaultPublicationConfig(title, summary, coverImageURL string) (datatypes.JSON, error) {
	digest := strings.TrimSpace(summary)
	if digest == "" {
		digest = title
	}
	config := map[string]any{
		"digest": TruncateRunes(digest, 120),
		"title":  title,
	}
	if coverImageURL := strings.TrimSpace(coverImageURL); coverImageURL != "" {
		config["cover_image_url"] = coverImageURL
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func TruncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (s *Service) ListProjects(page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	return s.ListProjectsCursor("", page, limit, status, filterUserID, platform, scopeUserID)
}

func (s *Service) ListProjectsCursor(cursor string, page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	if s.canUseDashboardProjectListCache() {
		return s.getCachedDashboardProjectList(cursor, page, limit, status, filterUserID, platform, scopeUserID)
	}
	return s.computeProjectList(cursor, page, limit, status, filterUserID, platform, scopeUserID)
}

func (s *Service) CanUseDashboardProjectListCache() bool {
	return s.canUseDashboardProjectListCache()
}

func (s *Service) ListCachedWorkspaceProjects(workspaceID uuid.UUID, actorUserID uuid.UUID, cursor string, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	return s.getCachedWorkspaceProjectList(workspaceID, actorUserID, cursor, page, limit, status, platform)
}

func (s *Service) computeProjectList(cursor string, page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	if scopeUserID == nil && platform == "" && s.canUseReadModels() {
		if resp, ok, err := s.adminProjectListFromReadModel(cursor, page, limit, status, filterUserID); err != nil {
			return nil, err
		} else if ok {
			return resp, nil
		}
	}

	query := s.projectListReadDB(scopeUserID).Model(&models.Project{})

	// Apply scope (User dashboard enforces scopeUserID, overriding any filterUserID)
	if scopeUserID != nil {
		query = s.ScopeAccessibleProjects(query, *scopeUserID)
	} else if filterUserID != "" {
		// Admin dashboard can filter by specific user
		if uid, err := uuid.Parse(filterUserID); err == nil {
			query = query.Where("user_id = ?", uid)
		}
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if platform != "" {
		query = query.Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id").
			Where("ppp.platform = ?", platform).
			Group("projects.id")
	}

	return s.ListProjectPage(query, cursor, page, limit, scopeUserID)
}

func (s *Service) computeWorkspaceProjectList(workspaceID uuid.UUID, actorUserID uuid.UUID, cursor string, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	query := s.strongReadDB().Model(&models.Project{}).Where("workspace_id = ?", workspaceID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if platform != "" {
		query = query.Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id").
			Where("ppp.platform = ?", platform).
			Group("projects.id")
	}

	return s.ListProjectPage(query, cursor, page, limit, &actorUserID)
}

func (s *Service) adminProjectListFromReadModel(cursor string, page, limit int, status, filterUserID string) (*dto.PaginationResponse, bool, error) {
	query := s.projectListReadDB(nil).Model(&models.ProjectListSummary{})
	factQuery := s.projectListReadDB(nil).Model(&models.Project{})
	if filterUserID != "" {
		if uid, err := uuid.Parse(filterUserID); err == nil {
			query = query.Where("user_id = ?", uid)
			factQuery = factQuery.Where("user_id = ?", uid)
		}
	}
	if status != "" {
		query = query.Where("status = ?", status)
		factQuery = factQuery.Where("status = ?", status)
	}

	var summaryCount int64
	if err := query.Count(&summaryCount).Error; err != nil {
		return nil, false, err
	}
	if summaryCount == 0 {
		return nil, false, nil
	}
	var factCount int64
	if err := factQuery.Count(&factCount).Error; err != nil {
		return nil, false, err
	}
	if summaryCount != factCount {
		return nil, false, nil
	}

	resp, err := s.ListProjectSummaryPage(query, cursor, page, limit)
	if err != nil {
		return nil, false, err
	}
	return resp, true, nil
}

func (s *Service) projectListReadDB(scopeUserID *uuid.UUID) *gorm.DB {
	if scopeUserID != nil {
		return s.strongReadDB()
	}
	return s.eventualReadDB()
}

func (s *Service) ListProjectPage(query *gorm.DB, cursor string, page, limit int, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	var projects []models.Project

	page, limit = normalizeProjectListPage(page, limit)
	query, err := applyProjectListCursor(query, cursor)
	if err != nil {
		return nil, err
	}

	// Fetch data with specific fields and preload summary publications
	if err := query.Omit("source_content").
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, draft_status, review_status, sync_required, publish_url")
		}).
		Order("projects.created_at DESC").
		Order("projects.id ASC").
		Limit(limit + 1).
		Find(&projects).Error; err != nil {
		return nil, err
	}

	hasMore := len(projects) > limit
	if hasMore {
		projects = projects[:limit]
	}

	accessByProjectID := make(map[uuid.UUID]projectAccessResolution, len(projects))
	if scopeUserID != nil {
		var accessErr error
		accessByProjectID, accessErr = s.projectAccessForUser(projects, *scopeUserID)
		if accessErr != nil {
			return nil, accessErr
		}
	} else {
		for _, project := range projects {
			accessByProjectID[project.ID] = projectAccessResolution{
				role:   models.ProjectRoleOwner,
				source: models.ProjectAccessSourceOwner,
			}
		}
	}

	// Map to DTO
	items := make([]dto.ProjectListItem, 0, len(projects))
	for _, p := range projects {
		items = append(items, projectListItemFromModel(p, accessByProjectID[p.ID]))
	}

	total := int64((page-1)*limit + len(items))
	if hasMore {
		total++
	}
	totalPages := page
	if len(items) == 0 && page == 1 {
		totalPages = 0
	} else if hasMore {
		totalPages = page + 1
	}

	nextCursor := ""
	if hasMore && len(projects) > 0 {
		nextCursor = encodeProjectListCursor(projects[len(projects)-1])
	}

	return &dto.PaginationResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		Cursor:     strings.TrimSpace(cursor),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *Service) ListProjectSummaryPage(query *gorm.DB, cursor string, page, limit int) (*dto.PaginationResponse, error) {
	var summaries []models.ProjectListSummary

	page, limit = normalizeProjectListPage(page, limit)
	query, err := applyProjectListCursorColumns(query, cursor, "project_list_summaries.created_at", "project_list_summaries.project_id")
	if err != nil {
		return nil, err
	}

	if err := query.
		Order("project_list_summaries.created_at DESC").
		Order("project_list_summaries.project_id ASC").
		Limit(limit + 1).
		Find(&summaries).Error; err != nil {
		return nil, err
	}

	hasMore := len(summaries) > limit
	if hasMore {
		summaries = summaries[:limit]
	}

	items := make([]dto.ProjectListItem, 0, len(summaries))
	for _, summary := range summaries {
		item, err := projectListItemFromSummary(summary, projectAccessResolution{
			role:   models.ProjectRoleOwner,
			source: models.ProjectAccessSourceOwner,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	total := int64((page-1)*limit + len(items))
	if hasMore {
		total++
	}
	totalPages := page
	if len(items) == 0 && page == 1 {
		totalPages = 0
	} else if hasMore {
		totalPages = page + 1
	}

	nextCursor := ""
	if hasMore && len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		nextCursor = encodeProjectListCursorValues(last.CreatedAt, last.ProjectID)
	}

	return &dto.PaginationResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		Cursor:     strings.TrimSpace(cursor),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
