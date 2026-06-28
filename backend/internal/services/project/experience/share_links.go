package experience

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	"github.com/kurodakayn/mpp-backend/internal/services/project/projecterr"
)

const projectShareTokenBytes = 32

func (s *Service) ListProjectShareLinks(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectShareLinksResponse, error) {
	project, err := s.requireProjectOwner(projectID, userID)
	if err != nil {
		return nil, err
	}
	var links []models.ProjectShareLink
	if err := s.db.Where("workspace_id = ? AND project_id = ?", models.ProjectWorkspaceID(*project), projectID).Order("created_at desc").Find(&links).Error; err != nil {
		return nil, err
	}
	items := make([]dto.ProjectShareLink, 0, len(links))
	for _, link := range links {
		items = append(items, projectShareLinkFromModel(link))
	}
	return &dto.ProjectShareLinksResponse{Items: items}, nil
}

func (s *Service) CreateProjectShareLink(projectID uuid.UUID, userID uuid.UUID, req dto.CreateProjectShareLinkRequest, baseURL string) (*dto.ProjectShareLinkWithToken, error) {
	project, err := s.requireProjectOwner(projectID, userID)
	if err != nil {
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
		WorkspaceID: models.ProjectWorkspaceID(*project),
		ProjectID:   projectID,
		CreatedBy:   userID,
		TokenHash:   hashProjectShareToken(token),
		Role:        role,
		Status:      models.ProjectShareLinkStatusActive,
		ExpiresAt:   req.ExpiresAt,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
		return RecordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityShareLinkCreated, map[string]any{
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
		if models.ProjectWorkspaceID(project) != link.WorkspaceID {
			return gorm.ErrRecordNotFound
		}
		projectID = project.ID

		if project.UserID == userID {
			return nil
		}

		role := link.Role
		effectiveRole, err := accesspolicy.ProjectAccessRoleWithDB(tx, project, userID)
		if err == nil && projectRoleRank(effectiveRole) >= projectRoleRank(role) {
			return nil
		}
		if err != nil && !errors.Is(err, accesspolicy.ErrForbidden) {
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

		return RecordProjectActivity(tx, project.ID, userID, nil, models.ProjectActivityShareLinkAccepted, map[string]any{
			"role":          role,
			"share_link_id": link.ID.String(),
		})
	}); err != nil {
		return nil, err
	}

	if collaboratorChanged {
		s.invalidateDashboardCaches(false)
		s.invalidateScopedStatsCache()
	}

	project, err := s.getProject(projectID, &userID)
	if err != nil {
		return nil, err
	}

	return &dto.AcceptProjectShareLinkResponse{
		Project: project,
		Role:    project.Role,
	}, nil
}

func (s *Service) RevokeProjectShareLink(projectID uuid.UUID, userID uuid.UUID, linkID uuid.UUID) error {
	project, err := s.requireProjectOwner(projectID, userID)
	if err != nil {
		return err
	}
	if linkID == uuid.Nil {
		return ErrInvalidProjectShareLink
	}
	workspaceID := models.ProjectWorkspaceID(*project)
	now := time.Now().UTC()
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.ProjectShareLink{}).
			Where("id = ? AND workspace_id = ? AND project_id = ? AND status = ?", linkID, workspaceID, projectID, models.ProjectShareLinkStatusActive).
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
		return RecordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityShareLinkRevoked, map[string]any{
			"share_link_id": linkID.String(),
		})
	})
}

func (s *Service) requireProjectOwner(projectID uuid.UUID, actorUserID uuid.UUID) (*models.Project, error) {
	if projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, projecterr.ErrInvalidProject
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != actorUserID {
		return nil, accesspolicy.ErrForbidden
	}
	return &project, nil
}

func normalizeProjectCollaboratorRole(role string) (string, error) {
	role = strings.TrimSpace(role)
	switch role {
	case models.ProjectRoleEditor, models.ProjectRoleViewer:
		return role, nil
	default:
		return "", projecterr.ErrInvalidProjectCollaborator
	}
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
