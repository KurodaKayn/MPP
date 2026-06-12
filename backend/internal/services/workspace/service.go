package workspace

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/models"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidWorkspace = errors.New("invalid workspace")
var ErrInvalidWorkspaceMember = errors.New("invalid workspace member")
var ErrInvalidWorkspaceInvite = errors.New("invalid workspace invite")

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

type Service struct {
	db         *gorm.DB
	router     *dbrouter.Router
	projects   *projectsvc.Service
	readModels DashboardReadModelUpdater
}

type DashboardReadModelUpdater interface {
	RefreshProjectAsync(ctx context.Context, projectID uuid.UUID)
	RefreshWorkspaceAsync(ctx context.Context, workspaceID uuid.UUID)
}

func RoleHasPermission(role string, permission Permission) bool {
	permissions, ok := rolePermissions[role]
	if !ok {
		return false
	}
	_, ok = permissions[permission]
	return ok
}

func (s *Service) WorkspaceAccessRole(workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	return s.workspaceAccessRole(workspaceID, userID)
}

func (s *Service) RequirePermission(workspaceID uuid.UUID, userID uuid.UUID, permission Permission) (string, error) {
	role, err := s.workspaceAccessRole(workspaceID, userID)
	if err != nil {
		return "", err
	}
	if !RoleHasPermission(role, permission) {
		return "", ErrForbidden
	}
	return role, nil
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	return NewServiceWithRouter(db, projects, nil)
}

func NewServiceWithRouter(db *gorm.DB, projects *projectsvc.Service, router *dbrouter.Router) *Service {
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	return &Service{db: db, router: router, projects: projects}
}

func (s *Service) SetDashboardReadModelUpdater(updater DashboardReadModelUpdater) {
	s.readModels = updater
}

func (s *Service) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

func (s *Service) strongReadDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Reader(s.requestContext(), dbrouter.StrongRead)
}

func (s *Service) refreshWorkspaceReadModel(workspaceID uuid.UUID) {
	if s.readModels == nil || workspaceID == uuid.Nil {
		return
	}
	s.readModels.RefreshWorkspaceAsync(s.requestContext(), workspaceID)
}
