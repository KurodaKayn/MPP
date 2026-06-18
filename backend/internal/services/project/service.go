package project

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
	platformcapabilities "github.com/kurodakayn/mpp-backend/internal/platformcapabilities"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
)

var ErrForbidden = accesspolicy.ErrForbidden
var ErrInvalidProject = errors.New("invalid project")
var ErrInvalidProjectCollaborator = errors.New("invalid project collaborator")
var ErrProjectCollabUnavailable = errors.New("project collaboration unavailable")
var ErrProjectDeletionBlocked = errors.New("project deletion blocked")

const dashboardProjectListCacheTTL = 15 * time.Second

type DashboardStatsCacheInvalidator interface {
	InvalidateDashboardStatsCache(ctx context.Context)
	InvalidateDashboardScopedStatsCache(ctx context.Context)
}

type DashboardReadModelUpdater interface {
	RefreshProjectAsync(ctx context.Context, projectID uuid.UUID)
	RefreshWorkspaceAsync(ctx context.Context, workspaceID uuid.UUID)
}

var allowedProjectPlatforms = platformcapabilities.ProjectPlatformSet()

type Service struct {
	db                *gorm.DB
	router            *dbrouter.Router
	collabDocuments   *collabdoc.Service
	cache             *redis.Client
	cacheTTL          time.Duration
	cacheGroup        *singleflight.Group
	projectListGuard  *redisdegrade.Guard
	contentSetupGuard *redisdegrade.Guard
	statsCache        DashboardStatsCacheInvalidator
	readModels        DashboardReadModelUpdater
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithRouter(db, nil)
}

func NewServiceWithRouter(db *gorm.DB, router *dbrouter.Router) *Service {
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	return &Service{db: db, router: router, cacheGroup: &singleflight.Group{}}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	scoped.cacheGroup = s.cacheGroup
	if s.collabDocuments != nil {
		scoped.collabDocuments = s.collabDocuments.WithContext(ctx)
	}
	return &scoped
}

func (s *Service) SetCollabDocumentService(svc *collabdoc.Service) {
	s.collabDocuments = svc
}

func (s *Service) UseRedis(client *redis.Client) {
	s.UseRedisCache(client)
}

func (s *Service) UseRedisCache(client *redis.Client) {
	if client == nil {
		return
	}
	s.cache = client
	s.cacheTTL = dashboardProjectListCacheTTL
	if s.projectListGuard == nil {
		s.projectListGuard = redisdegrade.NewGuard(redisdegrade.GroupDashboardProjectListCache)
	}
	if s.contentSetupGuard == nil {
		s.contentSetupGuard = redisdegrade.NewGuard(redisdegrade.GroupDashboardContentSetupCache)
	}
}

func (s *Service) SetDashboardStatsCacheInvalidator(invalidator DashboardStatsCacheInvalidator) {
	s.statsCache = invalidator
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

func (s *Service) eventualReadDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Reader(s.requestContext(), dbrouter.EventualRead)
}

func (s *Service) strongReadDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Reader(s.requestContext(), dbrouter.StrongRead)
}

func (s *Service) canUseReadModels() bool {
	stickyUntil, sticky := dbrouter.StickyWriterUntil(s.requestContext())
	return !sticky || !stickyUntil.After(time.Now())
}

func ensurePersonalWorkspace(tx *gorm.DB, ownerUserID uuid.UUID) error {
	workspaceID := models.PersonalWorkspaceID(ownerUserID)
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerUserID,
		Name:        models.PersonalWorkspaceName,
		Slug:        models.PersonalWorkspaceSlug(ownerUserID),
		Status:      models.WorkspaceStatusActive,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&workspace).Error; err != nil {
		return err
	}

	now := time.Now()
	member := models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      ownerUserID,
		Role:        models.WorkspaceRoleOwner,
		JoinedAt:    &now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(&member).Error
}
