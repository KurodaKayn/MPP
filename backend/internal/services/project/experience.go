package project

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const projectShareTokenBytes = 32

var ErrInvalidProjectComment = projectError("invalid project comment")
var ErrInvalidProjectShareLink = projectError("invalid project share link")
var ErrInvalidProjectVersion = projectError("invalid project version")

type projectError string

func (e projectError) Error() string { return string(e) }

func (s *Service) ListProjectActivities(projectID uuid.UUID, userID uuid.UUID, limit int) (*dto.ProjectActivitiesResponse, error) {
	if err := s.requireProjectAccess(projectID, userID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var activities []models.ProjectActivity
	if err := s.db.
		Preload("Actor", selectUserIdentity).
		Preload("TargetUser", selectUserIdentity).
		Where("project_id = ?", projectID).
		Order("created_at desc").
		Order("id desc").
		Limit(limit).
		Find(&activities).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectActivity, 0, len(activities))
	for _, activity := range activities {
		items = append(items, projectActivityFromModel(activity))
	}
	return &dto.ProjectActivitiesResponse{Items: items}, nil
}

func (s *Service) ListProjectComments(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectCommentsResponse, error) {
	if err := s.requireProjectAccess(projectID, userID); err != nil {
		return nil, err
	}

	var comments []models.ProjectComment
	if err := s.db.
		Preload("Author", selectUserIdentity).
		Where("project_id = ?", projectID).
		Order("created_at desc").
		Find(&comments).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectComment, 0, len(comments))
	for _, comment := range comments {
		items = append(items, projectCommentFromModel(comment))
	}
	return &dto.ProjectCommentsResponse{Items: items}, nil
}

func (s *Service) CreateProjectComment(projectID uuid.UUID, userID uuid.UUID, req dto.CreateProjectCommentRequest) (*dto.ProjectComment, error) {
	if err := s.requireProjectAccess(projectID, userID); err != nil {
		return nil, err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, ErrInvalidProjectComment
	}

	metadata, err := jsonMap(req.Metadata)
	if err != nil {
		return nil, err
	}
	comment := models.ProjectComment{
		ProjectID:  projectID,
		AuthorID:   userID,
		Body:       body,
		AnchorText: strings.TrimSpace(req.AnchorText),
		Status:     models.ProjectCommentStatusOpen,
		Metadata:   metadata,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		return recordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityCommentCreated, map[string]any{
			"comment_id":  comment.ID.String(),
			"anchor_text": comment.AnchorText,
		})
	}); err != nil {
		return nil, err
	}
	return s.getProjectComment(comment.ID)
}

func (s *Service) UpdateProjectComment(projectID uuid.UUID, userID uuid.UUID, commentID uuid.UUID, req dto.UpdateProjectCommentRequest) (*dto.ProjectComment, error) {
	if projectID == uuid.Nil || commentID == uuid.Nil {
		return nil, ErrInvalidProjectComment
	}
	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := s.ProjectAccessRole(project, userID)
	if err != nil {
		return nil, err
	}
	if !CanEditProjectRole(role) {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(req.Status) != models.ProjectCommentStatusResolved {
		return nil, ErrInvalidProjectComment
	}

	now := time.Now().UTC()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.ProjectComment{}).
			Where("id = ? AND project_id = ?", commentID, projectID).
			Updates(map[string]any{
				"status":      models.ProjectCommentStatusResolved,
				"resolved_at": &now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return recordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityCommentResolved, map[string]any{
			"comment_id": commentID.String(),
		})
	}); err != nil {
		return nil, err
	}
	return s.getProjectComment(commentID)
}

func (s *Service) ListProjectVersions(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectVersionsResponse, error) {
	if err := s.requireProjectAccess(projectID, userID); err != nil {
		return nil, err
	}

	var versions []models.ProjectVersion
	if err := s.db.
		Preload("Creator", selectUserIdentity).
		Where("project_id = ?", projectID).
		Order("version_number desc").
		Find(&versions).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectVersion, 0, len(versions))
	for _, version := range versions {
		items = append(items, projectVersionFromModel(version))
	}
	return &dto.ProjectVersionsResponse{Items: items}, nil
}

func (s *Service) RestoreProjectVersion(projectID uuid.UUID, userID uuid.UUID, versionID uuid.UUID) (*dto.RestoreProjectVersionResponse, error) {
	if versionID == uuid.Nil {
		return nil, ErrInvalidProjectVersion
	}
	var version models.ProjectVersion
	if err := s.db.Preload("Creator", selectUserIdentity).First(&version, "id = ? AND project_id = ?", versionID, projectID).Error; err != nil {
		return nil, err
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
		previousCollabDocumentID := project.CollabDocumentID
		if err := tx.Model(&project).Updates(map[string]any{
			"title":              version.Title,
			"source_content":     version.SourceContent,
			"status":             models.ProjectStatusReady,
			"collab_document_id": nil,
		}).Error; err != nil {
			return err
		}
		project.Title = version.Title
		project.SourceContent = version.SourceContent
		project.Status = models.ProjectStatusReady
		project.CollabDocumentID = nil
		if err := createProjectVersion(tx, project, userID, "version_restore"); err != nil {
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
		return recordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityVersionRestored, map[string]any{
			"detached_collab_document_id": uuidString(previousCollabDocumentID),
			"version_id":                  version.ID.String(),
			"version_number":              version.VersionNumber,
		})
	}); err != nil {
		return nil, err
	}

	project, err := s.GetProject(projectID, &userID)
	if err != nil {
		return nil, err
	}
	versionDTO := projectVersionFromModel(version)
	return &dto.RestoreProjectVersionResponse{Project: project, Version: versionDTO}, nil
}

func (s *Service) ListProjectShareLinks(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectShareLinksResponse, error) {
	if _, err := s.requireProjectOwner(projectID, userID); err != nil {
		return nil, err
	}
	var links []models.ProjectShareLink
	if err := s.db.Where("project_id = ?", projectID).Order("created_at desc").Find(&links).Error; err != nil {
		return nil, err
	}
	items := make([]dto.ProjectShareLink, 0, len(links))
	for _, link := range links {
		items = append(items, projectShareLinkFromModel(link))
	}
	return &dto.ProjectShareLinksResponse{Items: items}, nil
}

func (s *Service) CreateProjectShareLink(projectID uuid.UUID, userID uuid.UUID, req dto.CreateProjectShareLinkRequest, baseURL string) (*dto.ProjectShareLinkWithToken, error) {
	if _, err := s.requireProjectOwner(projectID, userID); err != nil {
		return nil, err
	}
	role, err := normalizeProjectCollaboratorRole(req.Role)
	if err != nil {
		return nil, ErrInvalidProjectShareLink
	}
	token, err := newProjectShareToken()
	if err != nil {
		return nil, err
	}
	link := models.ProjectShareLink{
		ProjectID: projectID,
		CreatedBy: userID,
		TokenHash: hashProjectShareToken(token),
		Role:      role,
		Status:    models.ProjectShareLinkStatusActive,
		ExpiresAt: req.ExpiresAt,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
		return recordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityShareLinkCreated, map[string]any{
			"role": role,
		})
	}); err != nil {
		return nil, err
	}
	resp := projectShareLinkFromModel(link)
	return &dto.ProjectShareLinkWithToken{
		ProjectShareLink: resp,
		Token:            token,
		URL:              strings.TrimRight(baseURL, "/") + "/share/projects/" + token,
	}, nil
}

func (s *Service) AcceptProjectShareLink(token string, userID uuid.UUID) (*dto.AcceptProjectShareLinkResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" || userID == uuid.Nil {
		return nil, ErrInvalidProjectShareLink
	}

	tokenHash := hashProjectShareToken(token)
	now := time.Now().UTC()
	var projectID uuid.UUID
	collaboratorChanged := false

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var link models.ProjectShareLink
		if err := tx.
			Where("token_hash = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", tokenHash, models.ProjectShareLinkStatusActive, now).
			First(&link).Error; err != nil {
			return err
		}

		var project models.Project
		if err := tx.Select("id", "user_id", "workspace_id").First(&project, "id = ?", link.ProjectID).Error; err != nil {
			return err
		}
		projectID = project.ID

		if project.UserID == userID {
			return nil
		}

		role := link.Role
		effectiveRole, err := ProjectAccessRoleWithDB(tx, project, userID)
		if err == nil && projectRoleRank(effectiveRole) >= projectRoleRank(role) {
			return nil
		}
		if err != nil && !errors.Is(err, ErrForbidden) {
			return err
		}

		collaborator := models.ProjectCollaborator{
			ProjectID: project.ID,
			UserID:    userID,
			Role:      role,
			CreatedBy: link.CreatedBy,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "project_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"role":       role,
				"created_by": link.CreatedBy,
			}),
		}).Create(&collaborator).Error; err != nil {
			return err
		}
		collaboratorChanged = true

		return recordProjectActivity(tx, project.ID, userID, nil, models.ProjectActivityShareLinkAccepted, map[string]any{
			"role":          role,
			"share_link_id": link.ID.String(),
		})
	}); err != nil {
		return nil, err
	}

	if collaboratorChanged {
		s.invalidateDashboardCaches(false)
	}

	project, err := s.GetProject(projectID, &userID)
	if err != nil {
		return nil, err
	}

	return &dto.AcceptProjectShareLinkResponse{
		Project: project,
		Role:    project.Role,
	}, nil
}

func (s *Service) RevokeProjectShareLink(projectID uuid.UUID, userID uuid.UUID, linkID uuid.UUID) error {
	if _, err := s.requireProjectOwner(projectID, userID); err != nil {
		return err
	}
	if linkID == uuid.Nil {
		return ErrInvalidProjectShareLink
	}
	now := time.Now().UTC()
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.ProjectShareLink{}).
			Where("id = ? AND project_id = ? AND status = ?", linkID, projectID, models.ProjectShareLinkStatusActive).
			Updates(map[string]any{
				"status":     models.ProjectShareLinkStatusRevoked,
				"revoked_at": &now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return recordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityShareLinkRevoked, map[string]any{
			"share_link_id": linkID.String(),
		})
	})
}

func (s *Service) requireProjectAccess(projectID uuid.UUID, userID uuid.UUID) error {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return ErrInvalidProject
	}
	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return err
	}
	_, err := s.ProjectAccessRole(project, userID)
	return err
}

func (s *Service) getProjectComment(commentID uuid.UUID) (*dto.ProjectComment, error) {
	var comment models.ProjectComment
	if err := s.db.Preload("Author", selectUserIdentity).First(&comment, "id = ?", commentID).Error; err != nil {
		return nil, err
	}
	item := projectCommentFromModel(comment)
	return &item, nil
}

func selectUserIdentity(db *gorm.DB) *gorm.DB {
	return db.Select("id", "username", "email")
}

func recordProjectActivity(tx *gorm.DB, projectID uuid.UUID, actorUserID uuid.UUID, targetUserID *uuid.UUID, eventType string, metadata map[string]any) error {
	if projectID == uuid.Nil || actorUserID == uuid.Nil || strings.TrimSpace(eventType) == "" {
		return nil
	}
	payload, err := jsonMap(metadata)
	if err != nil {
		return err
	}
	createdAt := time.Now().UTC()
	var latestCreatedAt time.Time
	if err := tx.
		Model(&models.ProjectActivity{}).
		Where("project_id = ?", projectID).
		Select("created_at").
		Order("created_at desc").
		Limit(1).
		Scan(&latestCreatedAt).Error; err != nil {
		return err
	}
	if !latestCreatedAt.IsZero() && !createdAt.After(latestCreatedAt) {
		createdAt = latestCreatedAt.Add(time.Nanosecond)
	}
	return tx.Create(&models.ProjectActivity{
		ProjectID:    projectID,
		ActorUserID:  actorUserID,
		TargetUserID: targetUserID,
		EventType:    eventType,
		Metadata:     payload,
		CreatedAt:    createdAt,
	}).Error
}

func createProjectVersion(tx *gorm.DB, project models.Project, userID uuid.UUID, source string) error {
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		First(&models.Project{}, "id = ?", project.ID).Error; err != nil {
		return err
	}

	var latestVersionNumber int
	if err := tx.
		Model(&models.ProjectVersion{}).
		Select("COALESCE(MAX(version_number), 0)").
		Where("project_id = ?", project.ID).
		Scan(&latestVersionNumber).Error; err != nil {
		return err
	}
	collabSeq := int64(0)
	if project.CollabDocumentID != nil && *project.CollabDocumentID != uuid.Nil {
		var document models.CollabDocument
		if err := tx.Select("current_seq").First(&document, "id = ?", *project.CollabDocumentID).Error; err == nil {
			collabSeq = document.CurrentSeq
		}
	}
	return tx.Create(&models.ProjectVersion{
		ProjectID:        project.ID,
		CreatedBy:        userID,
		VersionNumber:    latestVersionNumber + 1,
		Title:            project.Title,
		SourceContent:    project.SourceContent,
		CollabDocumentID: project.CollabDocumentID,
		CollabSeq:        collabSeq,
		Source:           source,
	}).Error
}

func jsonMap(value map[string]any) (datatypes.JSON, error) {
	if value == nil {
		return datatypes.JSON([]byte(`{}`)), nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func mapFromJSON(value datatypes.JSON) map[string]any {
	var parsed map[string]any
	if err := json.Unmarshal(value, &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func newProjectShareToken() (string, error) {
	raw := make([]byte, projectShareTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashProjectShareToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func uuidString(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func projectRoleRank(role string) int {
	switch role {
	case models.ProjectRoleOwner:
		return 3
	case models.ProjectRoleEditor:
		return 2
	case models.ProjectRoleViewer:
		return 1
	default:
		return 0
	}
}

func projectActivityFromModel(activity models.ProjectActivity) dto.ProjectActivity {
	item := dto.ProjectActivity{
		ID:            activity.ID,
		ProjectID:     activity.ProjectID,
		ActorUserID:   activity.ActorUserID,
		ActorUsername: activity.Actor.Username,
		ActorEmail:    activity.Actor.Email,
		TargetUserID:  activity.TargetUserID,
		EventType:     activity.EventType,
		Metadata:      mapFromJSON(activity.Metadata),
		CreatedAt:     activity.CreatedAt,
	}
	if activity.TargetUser != nil {
		item.TargetUsername = activity.TargetUser.Username
		item.TargetEmail = activity.TargetUser.Email
	}
	return item
}

func projectCommentFromModel(comment models.ProjectComment) dto.ProjectComment {
	return dto.ProjectComment{
		ID:             comment.ID,
		ProjectID:      comment.ProjectID,
		AuthorID:       comment.AuthorID,
		AuthorUsername: comment.Author.Username,
		AuthorEmail:    comment.Author.Email,
		Body:           comment.Body,
		AnchorText:     comment.AnchorText,
		Status:         comment.Status,
		Metadata:       mapFromJSON(comment.Metadata),
		CreatedAt:      comment.CreatedAt,
		ResolvedAt:     comment.ResolvedAt,
	}
}

func projectVersionFromModel(version models.ProjectVersion) dto.ProjectVersion {
	return dto.ProjectVersion{
		ID:               version.ID,
		ProjectID:        version.ProjectID,
		CreatedBy:        version.CreatedBy,
		CreatorUsername:  version.Creator.Username,
		CreatorEmail:     version.Creator.Email,
		VersionNumber:    version.VersionNumber,
		Title:            version.Title,
		Source:           version.Source,
		CollabDocumentID: version.CollabDocumentID,
		CollabSeq:        version.CollabSeq,
		CreatedAt:        version.CreatedAt,
	}
}

func projectShareLinkFromModel(link models.ProjectShareLink) dto.ProjectShareLink {
	return dto.ProjectShareLink{
		ID:        link.ID,
		ProjectID: link.ProjectID,
		CreatedBy: link.CreatedBy,
		Role:      link.Role,
		Status:    link.Status,
		ExpiresAt: link.ExpiresAt,
		CreatedAt: link.CreatedAt,
		RevokedAt: link.RevokedAt,
	}
}
