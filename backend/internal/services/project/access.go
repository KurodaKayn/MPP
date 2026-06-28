package project

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
)

func (s *Service) ScopeAccessibleProjects(query *gorm.DB, userID uuid.UUID) *gorm.DB {
	return accesspolicy.ScopeAccessibleProjectsWithDB(s.db, query, userID)
}

func (s *Service) ProjectAccessRole(project models.Project, userID uuid.UUID) (string, error) {
	role, _, err := s.ProjectAccessRoleAndSource(project, userID)
	return role, err
}

func (s *Service) ProjectAccessRoleAndSource(project models.Project, userID uuid.UUID) (string, string, error) {
	return ProjectAccessRoleAndSourceWithDB(s.db, project, userID)
}

func (s *Service) WorkspaceProjectRole(workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	return accesspolicy.WorkspaceProjectRoleWithDB(s.db, workspaceID, userID)
}

func ProjectAccessRoleWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, error) {
	if userID == uuid.Nil {
		return "", ErrInvalidProject
	}
	return accesspolicy.ProjectAccessRoleWithDB(db, project, userID)
}

func ProjectAccessRoleAndSourceWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, string, error) {
	if userID == uuid.Nil {
		return "", "", ErrInvalidProject
	}
	return accesspolicy.ProjectAccessRoleAndSourceWithDB(db, project, userID)
}

func CanEditProjectRole(role string) bool {
	return accesspolicy.CanEditProjectRole(role)
}

func projectRoleForWorkspaceRole(role string) (string, error) {
	return accesspolicy.ProjectRoleForWorkspaceRole(role)
}

func workspaceProjectAccessRoleWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	return accesspolicy.WorkspaceProjectRoleWithDB(db, workspaceID, userID)
}

func (s *Service) requireProjectOwner(projectID uuid.UUID, actorUserID uuid.UUID) (*models.Project, error) {
	if projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
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

func (s *Service) projectAccessForUser(projects []models.Project, userID uuid.UUID) (map[uuid.UUID]projectAccessResolution, error) {
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

func (s *Service) workspaceProjectRolesForUser(workspaceIDSet map[uuid.UUID]struct{}, userID uuid.UUID) (map[uuid.UUID]string, error) {
	roles := make(map[uuid.UUID]string, len(workspaceIDSet))
	if len(workspaceIDSet) == 0 {
		return roles, nil
	}

	workspaceIDs := make([]uuid.UUID, 0, len(workspaceIDSet))
	for workspaceID := range workspaceIDSet {
		workspaceIDs = append(workspaceIDs, workspaceID)
	}

	var ownedWorkspaces []models.Workspace
	if err := s.db.
		Select("id").
		Where("owner_user_id = ? AND id IN ?", userID, workspaceIDs).
		Find(&ownedWorkspaces).Error; err != nil {
		return nil, err
	}
	for _, workspace := range ownedWorkspaces {
		role, err := projectRoleForWorkspaceRole(models.WorkspaceRoleOwner)
		if err != nil {
			return nil, err
		}
		roles[workspace.ID] = role
	}

	var members []models.WorkspaceMember
	if err := s.db.
		Select("workspace_id", "role").
		Where("user_id = ? AND workspace_id IN ?", userID, workspaceIDs).
		Find(&members).Error; err != nil {
		return nil, err
	}
	for _, member := range members {
		if _, ok := roles[member.WorkspaceID]; ok {
			continue
		}
		role, err := projectRoleForWorkspaceRole(member.Role)
		if err != nil {
			return nil, err
		}
		roles[member.WorkspaceID] = role
	}
	return roles, nil
}
