package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = errors.New("invalid project")
var ErrInvalidProjectCollaborator = errors.New("invalid project collaborator")
var ErrInvalidWorkspace = errors.New("invalid workspace")
var ErrInvalidWorkspaceMember = errors.New("invalid workspace member")
var ErrProjectCollabUnavailable = errors.New("project collaboration unavailable")
var ErrPublicationDisabled = publishsvc.ErrPublicationDisabled
var ErrPublicationRequiresSync = publishsvc.ErrPublicationRequiresSync
var ErrManualPublishUnsupported = publishsvc.ErrManualPublishUnsupported
var ErrExtensionCallbackTokenInvalid = errors.New("invalid extension callback token")
var ErrExtensionCallbackTokenExpired = errors.New("expired extension callback token")

var allowedProjectPlatforms = map[string]struct{}{
	"douyin": {},
	"wechat": {},
	"x":      {},
	"zhihu":  {},
}

const (
	extensionDouyinAdapterKey     = "DYNAMIC_DOUYIN"
	extensionArticleContentKind   = "article"
	extensionPreviewLimit         = 80
	extensionHandoffSchemaVersion = 1
	extensionHandoffType          = "mpp.extension_publish_handoff"
	extensionDouyinInjectURL      = "https://creator.douyin.com/creator-micro/content/upload?default-tab=5"
	extensionHandoffTTL           = 10 * time.Minute
)

type DashboardService struct {
	db                    *gorm.DB
	accounts              *platformaccount.Service
	publisher             *publishsvc.Service
	browserWorkerClient   publisher.BrowserWorkerClient
	browserSessionService *browsersession.BrowserSessionService
	collabDocuments       *collabdoc.Service
	draftCompiler         ProjectDraftCompiler
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, platformaccount.WechatAPITester{}, platformaccount.XAPITester{})
}

func (s *DashboardService) WithContext(ctx context.Context) *DashboardService {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	if s.accounts != nil {
		scoped.accounts = s.accounts.WithContext(ctx)
	}
	if s.publisher != nil {
		scoped.publisher = s.publisher.WithContext(ctx)
	}
	if s.browserSessionService != nil {
		scoped.browserSessionService = s.browserSessionService.WithContext(ctx)
	}
	if s.collabDocuments != nil {
		scoped.collabDocuments = s.collabDocuments.WithContext(ctx)
	}
	return &scoped
}

func (s *DashboardService) SetBrowserWorkerClient(client publisher.BrowserWorkerClient) {
	s.browserWorkerClient = client
}

func (s *DashboardService) SetBrowserSessionService(svc *browsersession.BrowserSessionService) {
	s.browserSessionService = svc
}

func (s *DashboardService) SetCollabDocumentService(svc *collabdoc.Service) {
	s.collabDocuments = svc
}

func (s *DashboardService) SetDraftCompiler(compiler ProjectDraftCompiler) {
	s.draftCompiler = compiler
}

func NewDashboardServiceWithWechatTester(db *gorm.DB, tester platformaccount.WechatConnectionTester) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, tester, platformaccount.XAPITester{})
}

func NewDashboardServiceWithPlatformTesters(db *gorm.DB, tester platformaccount.WechatConnectionTester, xTester platformaccount.XConnectionTester) *DashboardService {
	accounts := platformaccount.NewServiceWithPlatformTesters(db, tester, xTester)
	return &DashboardService{
		db:            db,
		accounts:      accounts,
		publisher:     publishsvc.NewService(db, accounts),
		draftCompiler: newContentPipelineDraftCompiler(),
	}
}

func NewDashboardServiceWithXOAuth2Provider(db *gorm.DB, provider platformaccount.XOAuth2Provider) *DashboardService {
	accounts := platformaccount.NewServiceWithXOAuth2Provider(db, provider)
	return &DashboardService{
		db:            db,
		accounts:      accounts,
		publisher:     publishsvc.NewService(db, accounts),
		draftCompiler: newContentPipelineDraftCompiler(),
	}
}

func (s *DashboardService) SetPublishQueue(queue publishsvc.PublishQueue) {
	s.publisher.SetQueue(queue)
}

func (s *DashboardService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.accounts.UseRedis(client)
	s.publisher.UseRedis(client)
	if s.browserSessionService != nil {
		s.browserSessionService.UseRedis(client)
	}
}

func (s *DashboardService) scopeAccessibleProjects(query *gorm.DB, userID uuid.UUID) *gorm.DB {
	collaboratorProjectIDs := s.db.
		Model(&models.ProjectCollaborator{}).
		Select("project_id").
		Where("user_id = ?", userID)
	memberWorkspaceIDs := s.db.
		Model(&models.WorkspaceMember{}).
		Select("workspace_id").
		Where("user_id = ?", userID)
	ownedWorkspaceIDs := s.db.
		Model(&models.Workspace{}).
		Select("id").
		Where("owner_user_id = ?", userID)
	return query.Where(
		"projects.user_id = ? OR projects.id IN (?) OR projects.workspace_id IN (?) OR projects.workspace_id IN (?)",
		userID,
		collaboratorProjectIDs,
		memberWorkspaceIDs,
		ownedWorkspaceIDs,
	)
}

func (s *DashboardService) projectAccessRole(project models.Project, userID uuid.UUID) (string, error) {
	return projectAccessRoleWithDB(s.db, project, userID)
}

func projectAccessRoleWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, error) {
	if userID == uuid.Nil {
		return "", ErrInvalidProject
	}
	if project.UserID == userID {
		return models.ProjectRoleOwner, nil
	}

	var collaborator models.ProjectCollaborator
	if err := db.
		Select("project_id", "user_id", "role").
		Where("project_id = ? AND user_id = ?", project.ID, userID).
		First(&collaborator).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	} else {
		return collaborator.Role, nil
	}

	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return workspaceProjectAccessRoleWithDB(db, *project.WorkspaceID, userID)
	}
	return "", ErrForbidden
}

func canEditProjectRole(role string) bool {
	return role == models.ProjectRoleOwner || role == models.ProjectRoleEditor
}

func projectRoleForWorkspaceRole(role string) (string, error) {
	switch role {
	case models.WorkspaceRoleOwner, models.WorkspaceRoleAdmin, models.WorkspaceRoleMember:
		return models.ProjectRoleEditor, nil
	case models.WorkspaceRoleViewer:
		return models.ProjectRoleViewer, nil
	default:
		return "", ErrForbidden
	}
}

func workspaceProjectAccessRoleWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	var workspace models.Workspace
	if err := db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return "", err
	}
	if workspace.OwnerUserID == userID {
		return projectRoleForWorkspaceRole(models.WorkspaceRoleOwner)
	}

	var member models.WorkspaceMember
	if err := db.
		Select("workspace_id", "user_id", "role").
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrForbidden
		}
		return "", err
	}
	return projectRoleForWorkspaceRole(member.Role)
}

func projectCollabDocumentRole(role string) (string, error) {
	switch role {
	case models.ProjectRoleOwner, models.ProjectRoleEditor:
		return models.CollabDocumentRoleEditor, nil
	case models.ProjectRoleViewer:
		return models.CollabDocumentRoleViewer, nil
	default:
		return "", ErrForbidden
	}
}

func normalizeProjectCollaboratorRole(role string) (string, error) {
	role = strings.TrimSpace(role)
	switch role {
	case models.ProjectRoleEditor, models.ProjectRoleViewer:
		return role, nil
	default:
		return "", ErrInvalidProjectCollaborator
	}
}

func (s *DashboardService) requireProjectOwner(projectID uuid.UUID, actorUserID uuid.UUID) (*models.Project, error) {
	if projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}

	var project models.Project
	if err := s.db.Select("id", "user_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != actorUserID {
		return nil, ErrForbidden
	}
	return &project, nil
}

func (s *DashboardService) projectRolesForUser(projects []models.Project, userID uuid.UUID) (map[uuid.UUID]string, error) {
	roles := make(map[uuid.UUID]string, len(projects))
	sharedProjectIDs := make([]uuid.UUID, 0)
	workspaceIDs := make(map[uuid.UUID]struct{})
	for _, project := range projects {
		if project.UserID == userID {
			roles[project.ID] = models.ProjectRoleOwner
			continue
		}
		sharedProjectIDs = append(sharedProjectIDs, project.ID)
		if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
			workspaceIDs[*project.WorkspaceID] = struct{}{}
		}
	}

	if len(sharedProjectIDs) > 0 {
		var collaborators []models.ProjectCollaborator
		if err := s.db.
			Select("project_id", "role").
			Where("user_id = ? AND project_id IN ?", userID, sharedProjectIDs).
			Find(&collaborators).Error; err != nil {
			return nil, err
		}
		for _, collaborator := range collaborators {
			roles[collaborator.ProjectID] = collaborator.Role
		}
	}

	if len(workspaceIDs) == 0 {
		return roles, nil
	}

	workspaceRoles, err := s.workspaceProjectRolesForUser(workspaceIDs, userID)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if _, ok := roles[project.ID]; ok {
			continue
		}
		if project.WorkspaceID == nil {
			continue
		}
		if role, ok := workspaceRoles[*project.WorkspaceID]; ok {
			roles[project.ID] = role
		}
	}
	return roles, nil
}

func (s *DashboardService) GetStats(scopeUserID *uuid.UUID) (*dto.DashboardStatsResponse, error) {
	var stats dto.DashboardStatsResponse

	// Users count (Only admin should see total users)
	if scopeUserID == nil {
		if err := s.db.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
			return nil, err
		}
	} else {
		stats.TotalUsers = 1 // Scoped to self
	}

	// Projects count
	projQuery := s.db.Model(&models.Project{})
	if scopeUserID != nil {
		projQuery = s.scopeAccessibleProjects(projQuery, *scopeUserID)
	}
	if err := projQuery.Count(&stats.TotalProjects).Error; err != nil {
		return nil, err
	}

	// Published publications count
	pubPubQuery := s.db.Model(&models.ProjectPlatformPublication{}).Where("project_platform_publications.status = ?", models.PublicationStatusPublished)
	if scopeUserID != nil {
		pubPubQuery = pubPubQuery.Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
			Scopes(func(db *gorm.DB) *gorm.DB {
				return s.scopeAccessibleProjects(db, *scopeUserID)
			})
	}
	if err := pubPubQuery.Count(&stats.TotalPublishedPublications).Error; err != nil {
		return nil, err
	}

	// Failed publications count
	failPubQuery := s.db.Model(&models.ProjectPlatformPublication{}).Where("project_platform_publications.status = ?", models.PublicationStatusFailed)
	if scopeUserID != nil {
		failPubQuery = failPubQuery.Joins("JOIN projects ON projects.id = project_platform_publications.project_id").
			Scopes(func(db *gorm.DB) *gorm.DB {
				return s.scopeAccessibleProjects(db, *scopeUserID)
			})
	}
	if err := failPubQuery.Count(&stats.TotalFailedPublications).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *DashboardService) GetExtensionSession(userID uuid.UUID) (*dto.ExtensionSessionResponse, error) {
	var user models.User
	if err := s.db.Select("id", "username").First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}

	return &dto.ExtensionSessionResponse{
		Authenticated: true,
		User: dto.ExtensionSessionUser{
			ID:       user.ID,
			Username: user.Username,
		},
	}, nil
}

func (s *DashboardService) ListExtensionPrepublish(userID uuid.UUID) (*dto.ExtensionPrepublishResponse, error) {
	var projects []models.Project
	if err := s.db.
		Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id AND ppp.platform = ?", "douyin").
		Where("projects.user_id = ?", userID).
		Preload("Publications", "platform = ?", "douyin").
		Order("projects.updated_at desc").
		Find(&projects).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ExtensionPrepublishItem, 0, len(projects))
	for _, project := range projects {
		platforms := make([]dto.ExtensionPrepublishPlatform, 0, len(project.Publications))
		for _, publication := range project.Publications {
			platforms = append(platforms, extensionPrepublishPlatformFromPublication(publication))
		}
		if len(platforms) == 0 {
			continue
		}
		items = append(items, dto.ExtensionPrepublishItem{
			ProjectID: project.ID,
			Title:     project.Title,
			Status:    project.Status,
			UpdatedAt: project.UpdatedAt,
			Platforms: platforms,
		})
	}

	return &dto.ExtensionPrepublishResponse{Items: items}, nil
}

func extensionPrepublishPlatformFromPublication(publication models.ProjectPlatformPublication) dto.ExtensionPrepublishPlatform {
	return dto.ExtensionPrepublishPlatform{
		PublicationID: publication.ID,
		Platform:      publication.Platform,
		AdapterKey:    extensionDouyinAdapterKey,
		ContentKind:   extensionArticleContentKind,
		Status:        publication.Status,
		Enabled:       publication.Enabled,
		Preview:       extensionPrepublishPreview(publication.AdaptedContent),
	}
}

func extensionPrepublishPreview(raw datatypes.JSON) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}

	for _, key := range []string{"text", "markdown", "html", "summary"} {
		value, ok := payload[key].(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			return truncateRunes(value, extensionPreviewLimit)
		}
	}

	return ""
}

func (s *DashboardService) CreateExtensionHandoff(userID uuid.UUID, req dto.CreateExtensionHandoffRequest, callbackURL string) (*dto.ExtensionPublishHandoff, error) {
	if req.ProjectID == uuid.Nil || len(req.Platforms) == 0 {
		return nil, ErrInvalidProject
	}
	platforms, err := normalizeExtensionHandoffPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "title").First(&project, "id = ?", req.ProjectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	executionID := uuid.NewString()
	expiresAt := time.Now().UTC().Add(extensionHandoffTTL)
	handoffPlatforms := make([]dto.ExtensionHandoffPlatform, 0, len(platforms))
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			var publication models.ProjectPlatformPublication
			if err := tx.Where("project_id = ? AND platform = ?", project.ID, platform).First(&publication).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPublicationRequiresSync
				}
				return err
			}
			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				return ErrPublicationDisabled
			}
			adaptedContent, err := extensionHandoffAdaptedContent(publication.AdaptedContent)
			if err != nil {
				return err
			}
			callbackToken := uuid.NewString()
			if err := tx.Create(&models.ExtensionCallbackToken{
				ExecutionID: executionID,
				ProjectID:   project.ID,
				UserID:      userID,
				Platform:    platform,
				Token:       callbackToken,
				ExpiresAt:   expiresAt,
			}).Error; err != nil {
				return err
			}
			handoffPlatforms = append(handoffPlatforms, dto.ExtensionHandoffPlatform{
				Platform:       platform,
				AdapterKey:     extensionDouyinAdapterKey,
				InjectURL:      extensionDouyinInjectURL,
				ContentKind:    extensionArticleContentKind,
				AutoPublish:    false,
				RequiresReview: true,
				AdaptedContent: adaptedContent,
				Assets:         []dto.ExtensionHandoffAsset{},
				Callback: dto.ExtensionHandoffCallback{
					URL:   callbackURL,
					Token: callbackToken,
				},
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &dto.ExtensionPublishHandoff{
		SchemaVersion: extensionHandoffSchemaVersion,
		Type:          extensionHandoffType,
		ExecutionID:   executionID,
		ExpiresAt:     expiresAt,
		Project: dto.ExtensionHandoffProject{
			ID:    project.ID,
			Title: project.Title,
		},
		Platforms: handoffPlatforms,
	}, nil
}

func (s *DashboardService) RecordExtensionEvent(req dto.ExtensionEventCallbackRequest) (*dto.ExtensionEventCallbackResponse, error) {
	tokenValue := strings.TrimSpace(req.Token)
	eventID := strings.TrimSpace(req.EventID)
	platform := strings.TrimSpace(req.Platform)
	status := strings.TrimSpace(req.Status)
	if tokenValue == "" || eventID == "" || platform == "" || status == "" {
		return nil, ErrExtensionCallbackTokenInvalid
	}

	var token models.ExtensionCallbackToken
	if err := s.db.First(&token, "token = ?", tokenValue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExtensionCallbackTokenInvalid
		}
		return nil, err
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return nil, ErrExtensionCallbackTokenExpired
	}
	if token.Platform != platform {
		return nil, ErrExtensionCallbackTokenInvalid
	}

	var existing models.ExtensionExecutionEvent
	if err := s.db.First(&existing, "event_id = ?", eventID).Error; err == nil {
		return &dto.ExtensionEventCallbackResponse{Accepted: true, Duplicate: true}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	metadata := datatypes.JSON([]byte(`{}`))
	if req.Metadata != nil {
		payload, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, err
		}
		metadata = datatypes.JSON(payload)
	}

	if err := s.db.Create(&models.ExtensionExecutionEvent{
		CallbackTokenID: token.ID,
		ExecutionID:     token.ExecutionID,
		ProjectID:       token.ProjectID,
		UserID:          token.UserID,
		EventID:         eventID,
		Platform:        platform,
		Status:          status,
		Message:         strings.TrimSpace(req.Message),
		RemoteID:        strings.TrimSpace(req.RemoteID),
		PublishURL:      strings.TrimSpace(req.PublishURL),
		ErrorMessage:    strings.TrimSpace(req.ErrorMessage),
		Metadata:        metadata,
	}).Error; err != nil {
		return nil, err
	}

	return &dto.ExtensionEventCallbackResponse{Accepted: true, Duplicate: false}, nil
}

func normalizeExtensionHandoffPlatforms(input []string) ([]string, error) {
	seen := map[string]struct{}{}
	platforms := make([]string, 0, len(input))
	for _, raw := range input {
		platform := strings.TrimSpace(raw)
		if platform == "" {
			continue
		}
		if platform != "douyin" {
			return nil, ErrInvalidProject
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		platforms = append(platforms, platform)
	}
	if len(platforms) == 0 {
		return nil, ErrInvalidProject
	}
	return platforms, nil
}

func extensionHandoffAdaptedContent(raw datatypes.JSON) (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrPublicationRequiresSync
	}
	text, ok := payload["text"].(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return nil, ErrPublicationRequiresSync
	}
	return map[string]interface{}{
		"schema_version": extensionHandoffSchemaVersion,
		"format":         "text",
		"text":           text,
	}, nil
}

func (s *DashboardService) CreateProject(userID uuid.UUID, req dto.CreateProjectRequest) (*dto.ProjectListItem, error) {
	return s.createProjectWithWorkspace(userID, nil, req)
}

func (s *DashboardService) createProjectWithWorkspace(userID uuid.UUID, workspaceID *uuid.UUID, req dto.CreateProjectRequest) (*dto.ProjectListItem, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := strings.TrimSpace(req.SourceContent)
	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if title == "" || sourceContent == "" || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	project := models.Project{
		UserID:        userID,
		WorkspaceID:   workspaceID,
		Title:         title,
		SourceContent: sourceContent,
		Status:        models.ProjectStatusReady,
	}
	publications := make([]dto.PublicationSummary, 0, len(platforms))

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		for _, platform := range platforms {
			config, adaptedContent, status, err := buildPendingPublicationPayload(title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}

			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}

			publications = append(publications, dto.PublicationSummary{
				ID:       publication.ID,
				Platform: platform,
				Enabled:  publication.Enabled,
				Status:   publication.Status,
			})
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &dto.ProjectListItem{
		ID:               project.ID,
		UserID:           project.UserID,
		WorkspaceID:      project.WorkspaceID,
		CollabDocumentID: project.CollabDocumentID,
		Title:            project.Title,
		Status:           project.Status,
		Role:             models.ProjectRoleOwner,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     publications,
	}, nil
}

func (s *DashboardService) GetProject(projectID uuid.UUID, scopeUserID *uuid.UUID) (*dto.ProjectDetail, error) {
	var project models.Project
	if err := s.db.
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, publish_url").Order("platform asc")
		}).
		First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}

	role := models.ProjectRoleOwner
	if scopeUserID != nil {
		accessRole, err := s.projectAccessRole(project, *scopeUserID)
		if err != nil {
			return nil, err
		}
		role = accessRole
	}

	return projectDetailFromModel(project, role), nil
}

func (s *DashboardService) UpdateProject(projectID uuid.UUID, userID uuid.UUID, req dto.UpdateProjectRequest) (*dto.ProjectDetail, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := strings.TrimSpace(req.SourceContent)
	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if title == "" || sourceContent == "" || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		role, err := projectAccessRoleWithDB(tx, project, userID)
		if err != nil {
			return err
		}
		if !canEditProjectRole(role) {
			return ErrForbidden
		}

		project.Title = title
		project.SourceContent = sourceContent
		project.Status = models.ProjectStatusReady
		if err := tx.Save(&project).Error; err != nil {
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
				if err := tx.Model(&publication).Updates(map[string]interface{}{
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
			if err := tx.Model(&publication).Updates(map[string]interface{}{
				"config":          config,
				"enabled":         true,
				"error_message":   "",
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

			config, adaptedContent, status, err := buildPendingPublicationPayload(title, req.Summary, req.CoverImageURL)
			if err != nil {
				return err
			}
			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return s.GetProject(projectID, &userID)
}

func (s *DashboardService) SaveProjectContent(projectID uuid.UUID, userID uuid.UUID, req dto.SaveProjectContentRequest) (*dto.ProjectDetail, error) {
	title := strings.TrimSpace(req.Title)
	sourceContent := strings.TrimSpace(req.SourceContent)
	if title == "" || sourceContent == "" {
		return nil, ErrInvalidProject
	}

	var project models.Project
	if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.projectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !canEditProjectRole(role) {
		return nil, ErrForbidden
	}

	if err := s.db.Model(&project).Updates(map[string]interface{}{
		"source_content": sourceContent,
		"status":         models.ProjectStatusReady,
		"title":          title,
	}).Error; err != nil {
		return nil, err
	}

	return s.GetProject(projectID, &userID)
}

func (s *DashboardService) SaveProjectPlatforms(projectID uuid.UUID, userID uuid.UUID, req dto.SaveProjectPlatformsRequest) (*dto.ProjectDetail, error) {
	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil || len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		role, err := projectAccessRoleWithDB(tx, project, userID)
		if err != nil {
			return err
		}
		if !canEditProjectRole(role) {
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
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"enabled":       false,
					"error_message": "",
					"status":        models.PublicationStatusDisabled,
				}).Error; err != nil {
					return err
				}
				continue
			}

			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"enabled": true,
					"status":  models.PublicationStatusPending,
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

			config, adaptedContent, status, err := buildPendingPublicationPayload(project.Title, "", "")
			if err != nil {
				return err
			}
			publication := models.ProjectPlatformPublication{
				ProjectID:      project.ID,
				Platform:       platform,
				Enabled:        true,
				Status:         status,
				Config:         config,
				AdaptedContent: adaptedContent,
			}
			if err := tx.Create(&publication).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return s.GetProject(projectID, &userID)
}

func (s *DashboardService) CreateProjectCollabSession(projectID uuid.UUID, userID uuid.UUID) (*collabdoc.Session, error) {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if s.collabDocuments == nil {
		return nil, ErrProjectCollabUnavailable
	}

	documentID, documentRole, err := s.ensureProjectCollabDocument(projectID, userID)
	if err != nil {
		return nil, err
	}

	if err := s.collabDocuments.InitializeProjectDocument(s.requestContext(), documentID); err != nil {
		return nil, errors.Join(ErrProjectCollabUnavailable, err)
	}

	return s.collabDocuments.CreateAuthorizedSession(s.requestContext(), userID, documentID, documentRole)
}

func (s *DashboardService) ensureProjectCollabDocument(projectID uuid.UUID, userID uuid.UUID) (uuid.UUID, string, error) {
	var documentID uuid.UUID
	var documentRole string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}

		role, err := projectAccessRoleWithDB(tx, project, userID)
		if err != nil {
			return err
		}
		documentRole, err = projectCollabDocumentRole(role)
		if err != nil {
			return err
		}

		if project.CollabDocumentID != nil && *project.CollabDocumentID != uuid.Nil {
			documentID = *project.CollabDocumentID
			return nil
		}

		document := models.CollabDocument{
			OwnerUserID:   project.UserID,
			Title:         project.Title,
			Status:        models.CollabDocumentStatusActive,
			SchemaVersion: 1,
			CurrentSeq:    0,
		}
		if err := tx.Create(&document).Error; err != nil {
			return err
		}
		if err := tx.Model(&project).Update("collab_document_id", document.ID).Error; err != nil {
			return err
		}
		documentID = document.ID
		return nil
	})
	return documentID, documentRole, err
}

func (s *DashboardService) ListProjectCollaborators(projectID uuid.UUID, actorUserID uuid.UUID) (*dto.ProjectCollaboratorsResponse, error) {
	if _, err := s.requireProjectOwner(projectID, actorUserID); err != nil {
		return nil, err
	}

	var collaborators []models.ProjectCollaborator
	if err := s.db.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("project_id = ?", projectID).
		Order("created_at asc").
		Find(&collaborators).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectCollaborator, 0, len(collaborators))
	for _, collaborator := range collaborators {
		items = append(items, projectCollaboratorFromModel(collaborator))
	}
	return &dto.ProjectCollaboratorsResponse{Items: items}, nil
}

func (s *DashboardService) AddProjectCollaborator(projectID uuid.UUID, actorUserID uuid.UUID, req dto.AddProjectCollaboratorRequest) (*dto.ProjectCollaborator, error) {
	project, err := s.requireProjectOwner(projectID, actorUserID)
	if err != nil {
		return nil, err
	}

	role, err := normalizeProjectCollaboratorRole(req.Role)
	if err != nil {
		return nil, err
	}

	user, err := s.resolveProjectCollaboratorUser(req)
	if err != nil {
		return nil, err
	}
	if user.ID == project.UserID {
		return nil, ErrInvalidProjectCollaborator
	}

	collaborator := models.ProjectCollaborator{
		ProjectID: projectID,
		UserID:    user.ID,
		Role:      role,
		CreatedBy: actorUserID,
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "project_id"},
			{Name: "user_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"role":       role,
			"created_by": actorUserID,
		}),
	}).Create(&collaborator).Error; err != nil {
		return nil, err
	}

	return s.getProjectCollaborator(projectID, user.ID)
}

func (s *DashboardService) UpdateProjectCollaborator(projectID uuid.UUID, actorUserID uuid.UUID, targetUserID uuid.UUID, req dto.UpdateProjectCollaboratorRequest) (*dto.ProjectCollaborator, error) {
	project, err := s.requireProjectOwner(projectID, actorUserID)
	if err != nil {
		return nil, err
	}
	if targetUserID == uuid.Nil || targetUserID == project.UserID {
		return nil, ErrInvalidProjectCollaborator
	}

	role, err := normalizeProjectCollaboratorRole(req.Role)
	if err != nil {
		return nil, err
	}

	var collaborator models.ProjectCollaborator
	if err := s.db.Where("project_id = ? AND user_id = ?", projectID, targetUserID).First(&collaborator).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&collaborator).Update("role", role).Error; err != nil {
		return nil, err
	}

	return s.getProjectCollaborator(projectID, targetUserID)
}

func (s *DashboardService) RemoveProjectCollaborator(projectID uuid.UUID, actorUserID uuid.UUID, targetUserID uuid.UUID) error {
	project, err := s.requireProjectOwner(projectID, actorUserID)
	if err != nil {
		return err
	}
	if targetUserID == uuid.Nil || targetUserID == project.UserID {
		return ErrInvalidProjectCollaborator
	}

	result := s.db.Delete(&models.ProjectCollaborator{}, "project_id = ? AND user_id = ?", projectID, targetUserID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *DashboardService) resolveProjectCollaboratorUser(req dto.AddProjectCollaboratorRequest) (*models.User, error) {
	var user models.User
	if req.UserID != uuid.Nil {
		if err := s.db.Select("id", "username", "email").First(&user, "id = ?", req.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		return nil, ErrInvalidProjectCollaborator
	}
	if err := s.db.
		Select("id", "username", "email").
		Where("LOWER(email) = LOWER(?)", email).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *DashboardService) getProjectCollaborator(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectCollaborator, error) {
	var collaborator models.ProjectCollaborator
	if err := s.db.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		First(&collaborator).Error; err != nil {
		return nil, err
	}
	item := projectCollaboratorFromModel(collaborator)
	return &item, nil
}

func projectCollaboratorFromModel(collaborator models.ProjectCollaborator) dto.ProjectCollaborator {
	return dto.ProjectCollaborator{
		ProjectID: collaborator.ProjectID,
		UserID:    collaborator.UserID,
		Username:  collaborator.User.Username,
		Email:     collaborator.User.Email,
		Role:      collaborator.Role,
		CreatedBy: collaborator.CreatedBy,
		CreatedAt: collaborator.CreatedAt,
	}
}

func buildPendingPublicationPayload(title, summary, coverImageURL string) (datatypes.JSON, datatypes.JSON, string, error) {
	config, err := defaultPublicationConfig(title, summary, coverImageURL)
	if err != nil {
		return nil, nil, "", err
	}

	return config, datatypes.JSON([]byte(`{}`)), models.PublicationStatusPending, nil
}

func projectDetailFromModel(project models.Project, role string) *dto.ProjectDetail {
	publications := make([]dto.PublicationSummary, 0, len(project.Publications))
	for _, pub := range project.Publications {
		publications = append(publications, dto.PublicationSummary{
			ID:         pub.ID,
			Platform:   pub.Platform,
			Enabled:    pub.Enabled,
			Status:     pub.Status,
			PublishURL: pub.PublishURL,
		})
	}
	if publications == nil {
		publications = []dto.PublicationSummary{}
	}

	return &dto.ProjectDetail{
		ID:               project.ID,
		UserID:           project.UserID,
		WorkspaceID:      project.WorkspaceID,
		CollabDocumentID: project.CollabDocumentID,
		Title:            project.Title,
		SourceContent:    project.SourceContent,
		Status:           project.Status,
		Role:             role,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     publications,
	}
}

func projectListItemFromModel(project models.Project, role string) dto.ProjectListItem {
	publications := make([]dto.PublicationSummary, 0, len(project.Publications))
	for _, pub := range project.Publications {
		publications = append(publications, dto.PublicationSummary{
			ID:         pub.ID,
			Platform:   pub.Platform,
			Enabled:    pub.Enabled,
			Status:     pub.Status,
			PublishURL: pub.PublishURL,
		})
	}
	if publications == nil {
		publications = []dto.PublicationSummary{}
	}

	return dto.ProjectListItem{
		ID:               project.ID,
		UserID:           project.UserID,
		WorkspaceID:      project.WorkspaceID,
		CollabDocumentID: project.CollabDocumentID,
		Title:            project.Title,
		Status:           project.Status,
		Role:             role,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
		Publications:     publications,
	}
}

func normalizeProjectPlatforms(input []string) ([]string, error) {
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
	config := map[string]interface{}{
		"digest": truncateRunes(digest, 120),
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

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (s *DashboardService) ListProjects(page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	query := s.db.Model(&models.Project{})

	// Apply scope (User dashboard enforces scopeUserID, overriding any filterUserID)
	if scopeUserID != nil {
		query = s.scopeAccessibleProjects(query, *scopeUserID)
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

	return s.listProjectPage(query, page, limit, scopeUserID)
}

func (s *DashboardService) listProjectPage(query *gorm.DB, page, limit int, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	var projects []models.Project
	var total int64

	// Count total before pagination
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// Calculate pagination
	offset := (page - 1) * limit
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// Fetch data with specific fields and preload summary publications
	if err := query.Omit("source_content").
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, publish_url")
		}).
		Order("projects.created_at desc").
		Limit(limit).Offset(offset).
		Find(&projects).Error; err != nil {
		return nil, err
	}

	roles := make(map[uuid.UUID]string, len(projects))
	if scopeUserID != nil {
		var roleErr error
		roles, roleErr = s.projectRolesForUser(projects, *scopeUserID)
		if roleErr != nil {
			return nil, roleErr
		}
	} else {
		for _, project := range projects {
			roles[project.ID] = models.ProjectRoleOwner
		}
	}

	// Map to DTO
	items := make([]dto.ProjectListItem, 0, len(projects))
	for _, p := range projects {
		items = append(items, projectListItemFromModel(p, roles[p.ID]))
	}

	return &dto.PaginationResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *DashboardService) SyncProjectPrepublish(projectID uuid.UUID, userID uuid.UUID, req dto.SyncPrepublishRequest) (*dto.ProjectPublicationsResponse, error) {
	var project models.Project
	if err := s.db.Preload("Publications").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.projectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !canEditProjectRole(role) {
		return nil, ErrForbidden
	}

	platforms, err := normalizeProjectPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}
	if len(platforms) == 0 {
		for _, publication := range project.Publications {
			if publication.Enabled && publication.Status != models.PublicationStatusDisabled {
				platforms = append(platforms, publication.Platform)
			}
		}
	}
	if len(platforms) == 0 {
		return nil, ErrInvalidProject
	}

	publications, err := s.ensurePrepublishPublications(&project, platforms)
	if err != nil {
		return nil, err
	}

	draftCompiler := s.draftCompiler
	if draftCompiler == nil {
		draftCompiler = newContentPipelineDraftCompiler()
	}
	compiledDrafts, err := draftCompiler.CompileProjectDrafts(s.requestContext(), &project, publications, platforms)
	if err != nil {
		if markErr := s.markPrepublishCompileFailure(project.ID, platforms, err); markErr != nil {
			return nil, markErr
		}
		return s.GetProjectPublications(projectID, &userID, true)
	}

	if err := s.applyCompiledPrepublishDrafts(project.ID, platforms, compiledDrafts); err != nil {
		return nil, err
	}

	return s.GetProjectPublications(projectID, &userID, true)
}

func (s *DashboardService) ensurePrepublishPublications(project *models.Project, platforms []string) ([]models.ProjectPlatformPublication, error) {
	publications := make([]models.ProjectPlatformPublication, 0, len(platforms))
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			var publication models.ProjectPlatformPublication
			err := tx.Where("project_id = ? AND platform = ?", project.ID, platform).First(&publication).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				config, adaptedContent, status, err := buildPendingPublicationPayload(project.Title, "", "")
				if err != nil {
					return err
				}
				publication = models.ProjectPlatformPublication{
					ProjectID:      project.ID,
					Platform:       platform,
					Enabled:        true,
					Status:         status,
					Config:         config,
					AdaptedContent: adaptedContent,
				}
				if err := tx.Create(&publication).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				if err := tx.Model(&publication).Updates(map[string]interface{}{
					"enabled": true,
					"status":  models.PublicationStatusPending,
				}).Error; err != nil {
					return err
				}
				publication.Enabled = true
				publication.Status = models.PublicationStatusPending
			}

			publications = append(publications, publication)
		}

		return nil
	}); err != nil {
		return nil, err
	}
	return publications, nil
}

func (s *DashboardService) applyCompiledPrepublishDrafts(projectID uuid.UUID, platforms []string, drafts map[string][]byte) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			adaptedContent, ok := drafts[platform]
			if !ok {
				if err := tx.Model(&models.ProjectPlatformPublication{}).
					Where("project_id = ? AND platform = ?", projectID, platform).
					Updates(map[string]interface{}{
						"error_message": "content pipeline did not return a compiled draft",
						"status":        models.PublicationStatusFailed,
					}).Error; err != nil {
					return err
				}
				continue
			}

			if err := tx.Model(&models.ProjectPlatformPublication{}).
				Where("project_id = ? AND platform = ?", projectID, platform).
				Updates(map[string]interface{}{
					"adapted_content": datatypes.JSON(adaptedContent),
					"enabled":         true,
					"error_message":   "",
					"last_attempt_at": nil,
					"published_at":    nil,
					"publish_url":     "",
					"remote_id":       "",
					"retry_count":     0,
					"status":          models.PublicationStatusAdapted,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *DashboardService) markPrepublishCompileFailure(projectID uuid.UUID, platforms []string, err error) error {
	if len(platforms) == 0 {
		return nil
	}
	return s.db.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND platform IN ?", projectID, platforms).
		Updates(map[string]interface{}{
			"error_message": publishsvc.SanitizeUserFacingErrorMessage(err.Error()),
			"status":        models.PublicationStatusFailed,
		}).Error
}

func (s *DashboardService) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

func (s *DashboardService) UpdateProjectPrepublishDraft(projectID uuid.UUID, userID uuid.UUID, platform string, req dto.UpdatePrepublishDraftRequest) (*dto.ProjectPublicationsResponse, error) {
	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.projectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !canEditProjectRole(role) {
		return nil, ErrForbidden
	}

	platforms, err := normalizeProjectPlatforms([]string{platform})
	if err != nil || len(platforms) != 1 {
		return nil, ErrInvalidProject
	}
	if len(req.AdaptedContent) == 0 {
		return nil, ErrInvalidProject
	}

	adaptedContent, err := json.Marshal(req.AdaptedContent)
	if err != nil {
		return nil, err
	}

	var publication models.ProjectPlatformPublication
	if err := s.db.Where("project_id = ? AND platform = ?", projectID, platforms[0]).First(&publication).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&publication).Updates(map[string]interface{}{
		"adapted_content": datatypes.JSON(adaptedContent),
		"enabled":         true,
		"error_message":   "",
		"last_attempt_at": nil,
		"published_at":    nil,
		"publish_url":     "",
		"remote_id":       "",
		"retry_count":     0,
		"status":          models.PublicationStatusAdapted,
	}).Error; err != nil {
		return nil, err
	}

	return s.GetProjectPublications(projectID, &userID, true)
}

func (s *DashboardService) GetProjectPublications(projectID uuid.UUID, scopeUserID *uuid.UUID, includeContent bool) (*dto.ProjectPublicationsResponse, error) {
	// Verify project exists and access
	var proj models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").Where("id = ?", projectID).First(&proj).Error; err != nil {
		return nil, err
	}

	if scopeUserID != nil {
		if _, err := s.projectAccessRole(proj, *scopeUserID); err != nil {
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
		var rawConfig map[string]interface{}
		_ = json.Unmarshal(pub.Config, &rawConfig)
		safeConfig := filterConfig(rawConfig)

		// Safe parse adapted content
		var rawContent map[string]interface{}
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

func filterConfig(raw map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{})
	allowedKeys := []string{"title", "tags", "cover_image", "topics", "category", "original_declaration", "username"}
	for _, key := range allowedKeys {
		if val, ok := raw[key]; ok {
			safe[key] = val
		}
	}
	return safe
}

func summarizeAdaptedContent(raw map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{})
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
