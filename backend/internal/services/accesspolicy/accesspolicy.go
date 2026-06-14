package accesspolicy

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var ErrForbidden = errors.New("forbidden: you do not have permission to access this resource")

type Permission string

const (
	PermissionManageBilling   Permission = "workspace.manage_billing"
	PermissionManageMembers   Permission = "workspace.manage_members"
	PermissionAccountConnect  Permission = "account.connect"
	PermissionAccountManage   Permission = "account.manage"
	PermissionAccountUse      Permission = "account.use"
	PermissionProjectCreate   Permission = "project.create"
	PermissionProjectEdit     Permission = "project.edit"
	PermissionProjectReview   Permission = "project.review"
	PermissionPublishApprove  Permission = "publication.approve"
	PermissionPublishPublish  Permission = "publication.publish"
	PermissionPublishSchedule Permission = "publication.schedule"
)

var rolePermissions = map[string]map[Permission]struct{}{
	models.WorkspaceRoleOwner: {
		PermissionManageBilling: {}, PermissionManageMembers: {}, PermissionAccountConnect: {},
		PermissionAccountManage: {}, PermissionAccountUse: {}, PermissionProjectCreate: {},
		PermissionProjectEdit: {}, PermissionProjectReview: {}, PermissionPublishApprove: {},
		PermissionPublishPublish: {}, PermissionPublishSchedule: {},
	},
	models.WorkspaceRoleAdmin: {
		PermissionManageMembers: {}, PermissionAccountConnect: {}, PermissionAccountManage: {},
		PermissionAccountUse: {}, PermissionProjectCreate: {}, PermissionProjectEdit: {},
		PermissionProjectReview: {}, PermissionPublishApprove: {}, PermissionPublishPublish: {},
		PermissionPublishSchedule: {},
	},
	models.WorkspaceRoleMember: {
		PermissionAccountUse: {}, PermissionProjectCreate: {}, PermissionProjectEdit: {},
		PermissionProjectReview: {}, PermissionPublishPublish: {}, PermissionPublishSchedule: {},
	},
	models.WorkspaceRoleViewer: {
		PermissionProjectReview: {},
	},
}

func RoleHasPermission(role string, permission Permission) bool {
	permissions, ok := rolePermissions[role]
	if !ok {
		return false
	}
	_, ok = permissions[permission]
	return ok
}

func WorkspaceAccessRoleWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	if workspaceID == uuid.Nil || userID == uuid.Nil {
		return "", ErrForbidden
	}

	var workspace models.Workspace
	if err := db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return "", err
	}
	if workspace.OwnerUserID == userID {
		return models.WorkspaceRoleOwner, nil
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
	return member.Role, nil
}

func RequireWorkspacePermissionWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID, permission Permission) (string, error) {
	role, err := WorkspaceAccessRoleWithDB(db, workspaceID, userID)
	if err != nil {
		return "", err
	}
	if !RoleHasPermission(role, permission) {
		return "", ErrForbidden
	}
	return role, nil
}

func RequireWorkspaceMemberWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) error {
	if workspaceID == uuid.Nil || userID == uuid.Nil {
		return ErrForbidden
	}
	if workspaceID == models.PersonalWorkspaceID(userID) {
		return nil
	}
	_, err := WorkspaceAccessRoleWithDB(db, workspaceID, userID)
	return err
}

func RequireWorkspaceMemberRecordWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) error {
	if workspaceID == uuid.Nil || userID == uuid.Nil {
		return ErrForbidden
	}
	if workspaceID == models.PersonalWorkspaceID(userID) {
		return nil
	}
	var member models.WorkspaceMember
	if err := db.
		Select("workspace_id", "user_id", "role").
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrForbidden
		}
		return err
	}
	return nil
}

func RequireWorkspaceAccountConnectWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) error {
	if workspaceID == uuid.Nil {
		workspaceID = models.PersonalWorkspaceID(userID)
	}
	if workspaceID == models.PersonalWorkspaceID(userID) {
		return nil
	}
	_, err := RequireWorkspacePermissionWithDB(db, workspaceID, userID, PermissionAccountConnect)
	return err
}

func ScopeAccessibleProjectsWithDB(db *gorm.DB, query *gorm.DB, userID uuid.UUID) *gorm.DB {
	collaboratorProjectIDs := db.
		Model(&models.ProjectCollaborator{}).
		Select("project_id").
		Where("user_id = ?", userID)
	memberWorkspaceIDs := db.
		Model(&models.WorkspaceMember{}).
		Select("workspace_id").
		Where("user_id = ?", userID)
	ownedWorkspaceIDs := db.
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

func ProjectAccessRoleWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, error) {
	role, _, err := ProjectAccessRoleAndSourceWithDB(db, project, userID)
	return role, err
}

func ProjectAccessRoleAndSourceWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (string, string, error) {
	if userID == uuid.Nil {
		return "", "", ErrForbidden
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
		role, err := WorkspaceProjectRoleWithDB(db, *project.WorkspaceID, userID)
		return role, models.ProjectAccessSourceWorkspace, err
	}
	return "", "", ErrForbidden
}

func CanEditProjectRole(role string) bool {
	return role == models.ProjectRoleOwner || role == models.ProjectRoleEditor
}

func WorkspaceProjectRoleWithDB(db *gorm.DB, workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	workspaceRole, err := WorkspaceAccessRoleWithDB(db, workspaceID, userID)
	if err != nil {
		return "", err
	}
	return ProjectRoleForWorkspaceRole(workspaceRole)
}

func ProjectRoleForWorkspaceRole(role string) (string, error) {
	switch role {
	case models.WorkspaceRoleOwner, models.WorkspaceRoleAdmin, models.WorkspaceRoleMember:
		return models.ProjectRoleEditor, nil
	case models.WorkspaceRoleViewer:
		return models.ProjectRoleViewer, nil
	default:
		return "", ErrForbidden
	}
}

func ProjectForPublishWithDB(db *gorm.DB, projectID uuid.UUID, userID uuid.UUID) (models.Project, error) {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return models.Project{}, ErrForbidden
	}

	var project models.Project
	if err := db.First(&project, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Project{}, ErrForbidden
		}
		return models.Project{}, err
	}
	if project.UserID == userID {
		return project, nil
	}
	if ok, err := CanPublishProjectWithDB(db, project, userID); err != nil {
		return models.Project{}, err
	} else if !ok {
		return models.Project{}, ErrForbidden
	}
	return project, nil
}

func CanPublishProjectWithDB(db *gorm.DB, project models.Project, userID uuid.UUID) (bool, error) {
	var collaborator models.ProjectCollaborator
	if err := db.Select("role").
		First(&collaborator, "project_id = ? AND user_id = ?", project.ID, userID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	} else {
		return collaborator.Role == models.ProjectRoleOwner, nil
	}

	if project.WorkspaceID == nil || *project.WorkspaceID == uuid.Nil {
		return false, nil
	}
	role, err := WorkspaceAccessRoleWithDB(db, *project.WorkspaceID, userID)
	if errors.Is(err, ErrForbidden) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return role == models.WorkspaceRoleOwner || role == models.WorkspaceRoleAdmin, nil
}
