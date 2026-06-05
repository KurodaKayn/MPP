package dashboard

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/gorm"
)

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
	role, _, err := s.projectAccessRoleAndSource(project, userID)
	return role, err
}

func (s *DashboardService) projectAccessRoleAndSource(project models.Project, userID uuid.UUID) (string, string, error) {
	return projectAccessRoleAndSourceWithDB(s.db, project, userID)
}

func projectAccessRoleWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, error) {
	role, _, err := projectAccessRoleAndSourceWithDB(db, project, userID)
	return role, err
}

func projectAccessRoleAndSourceWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, string, error) {
	if userID == uuid.Nil {
		return "", "", ErrInvalidProject
	}
	if project.UserID == userID {
		return models.ProjectRoleOwner, models.ProjectAccessSourceOwner, nil
	}

	var collaborator models.ProjectCollaborator
	if err := db.
		Select("project_id", "user_id", "role").
		Where("project_id = ? AND user_id = ?", project.ID, userID).
		First(&collaborator).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", err
		}
	} else {
		return collaborator.Role, models.ProjectAccessSourceDirectShare, nil
	}

	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		role, err := workspaceProjectAccessRoleWithDB(db, *project.WorkspaceID, userID)
		return role, models.ProjectAccessSourceWorkspace, err
	}
	return "", "", ErrForbidden
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

type projectAccessResolution struct {
	role   string
	source string
}

func (s *DashboardService) projectAccessForUser(projects []models.Project, userID uuid.UUID) (map[uuid.UUID]projectAccessResolution, error) {
	access := make(map[uuid.UUID]projectAccessResolution, len(projects))
	sharedProjectIDs := make([]uuid.UUID, 0)
	workspaceIDs := make(map[uuid.UUID]struct{})
	for _, project := range projects {
		if project.UserID == userID {
			access[project.ID] = projectAccessResolution{
				role:   models.ProjectRoleOwner,
				source: models.ProjectAccessSourceOwner,
			}
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
			access[collaborator.ProjectID] = projectAccessResolution{
				role:   collaborator.Role,
				source: models.ProjectAccessSourceDirectShare,
			}
		}
	}

	if len(workspaceIDs) == 0 {
		return access, nil
	}

	workspaceRoles, err := s.workspaceProjectRolesForUser(workspaceIDs, userID)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if _, ok := access[project.ID]; ok {
			continue
		}
		if project.WorkspaceID == nil {
			continue
		}
		if role, ok := workspaceRoles[*project.WorkspaceID]; ok {
			access[project.ID] = projectAccessResolution{
				role:   role,
				source: models.ProjectAccessSourceWorkspace,
			}
		}
	}
	return access, nil
}
