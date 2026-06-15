package workspace

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

var ErrForbidden = accesspolicy.ErrForbidden
var ErrInvalidWorkspace = errors.New("invalid workspace")
var ErrInvalidWorkspaceMember = errors.New("invalid workspace member")
var ErrInvalidWorkspaceInvite = errors.New("invalid workspace invite")

type Permission = accesspolicy.Permission

const (
	PermissionManageBilling   = accesspolicy.PermissionManageBilling
	PermissionManageMembers   = accesspolicy.PermissionManageMembers
	PermissionAccountConnect  = accesspolicy.PermissionAccountConnect
	PermissionAccountManage   = accesspolicy.PermissionAccountManage
	PermissionAccountUse      = accesspolicy.PermissionAccountUse
	PermissionProjectCreate   = accesspolicy.PermissionProjectCreate
	PermissionProjectEdit     = accesspolicy.PermissionProjectEdit
	PermissionProjectReview   = accesspolicy.PermissionProjectReview
	PermissionPublishApprove  = accesspolicy.PermissionPublishApprove
	PermissionPublishPublish  = accesspolicy.PermissionPublishPublish
	PermissionPublishSchedule = accesspolicy.PermissionPublishSchedule
)

type Service struct {
	db         *gorm.DB
	router     *dbrouter.Router
	projects   *projectsvc.Service
	readModels DashboardReadModelUpdater
	listCache  DashboardProjectListCacheInvalidator
}

type DashboardReadModelUpdater interface {
	RefreshProjectAsync(ctx context.Context, projectID uuid.UUID)
	RefreshWorkspaceAsync(ctx context.Context, workspaceID uuid.UUID)
}

type DashboardProjectListCacheInvalidator interface {
	InvalidateDashboardProjectListCache(ctx context.Context)
}

func RoleHasPermission(role string, permission Permission) bool {
	return accesspolicy.RoleHasPermission(role, permission)
}

func (s *Service) WorkspaceAccessRole(workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	return s.workspaceAccessRole(workspaceID, userID)
}

func (s *Service) RequirePermission(workspaceID uuid.UUID, userID uuid.UUID, permission Permission) (string, error) {
	if workspaceID == uuid.Nil || userID == uuid.Nil {
		return "", ErrInvalidWorkspace
	}
	return accesspolicy.RequireWorkspacePermissionWithDB(s.strongReadDB(), workspaceID, userID, permission)
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

func (s *Service) SetDashboardProjectListCacheInvalidator(invalidator DashboardProjectListCacheInvalidator) {
	s.listCache = invalidator
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

func (s *Service) invalidateDashboardProjectListCache() {
	if s.listCache == nil {
		return
	}
	s.listCache.InvalidateDashboardProjectListCache(s.requestContext())
}
