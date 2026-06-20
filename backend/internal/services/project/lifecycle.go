package project

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkghtml "github.com/kurodakayn/mpp-backend/internal/pkg/html"
	projectpresenter "github.com/kurodakayn/mpp-backend/internal/services/project/presenter"
	"github.com/kurodakayn/mpp-backend/internal/services/project/publicationselection"
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
	var publications []dto.PublicationSummary
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

		created, err := publicationselection.CreateSelected(tx, project.ID, platforms, pendingPublicationConfigForTemplate(title, req.Summary, req.CoverImageURL, template))
		if err != nil {
			return err
		}
		createdPublications = created
		publications = projectpresenter.PublicationSummariesFromModels(createdPublications)

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

	detail := projectpresenter.ProjectDetailFromModel(project, role, accessSource)
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

		publications, err := publicationselection.ReconcileSelected(tx, project.ID, platforms, publicationselection.ReconcileResetAll, pendingPublicationConfigForTemplate(title, req.Summary, req.CoverImageURL, template))
		if err != nil {
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
		if err := publicationselection.MarkDraftsStale(tx, project.ID); err != nil {
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

		publications, err := publicationselection.ReconcileSelected(tx, project.ID, platforms, publicationselection.ReconcileKeepActive, defaultPublicationConfigForProjectTitle(project.Title))
		if err != nil {
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

func sanitizeProjectSourceContent(sourceContent string) string {
	return pkghtml.SanitizeStoredHTML(strings.TrimSpace(sourceContent))
}

func projectWorkspaceID(project models.Project) uuid.UUID {
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return *project.WorkspaceID
	}
	return models.PersonalWorkspaceID(project.UserID)
}
