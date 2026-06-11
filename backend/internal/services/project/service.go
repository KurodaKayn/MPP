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
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = errors.New("invalid project")
var ErrInvalidProjectCollaborator = errors.New("invalid project collaborator")
var ErrProjectCollabUnavailable = errors.New("project collaboration unavailable")

const dashboardProjectListCacheTTL = 15 * time.Second

type DashboardStatsCacheInvalidator interface {
	InvalidateDashboardStatsCache(ctx context.Context)
}

var allowedProjectPlatforms = map[string]struct{}{
	"douyin": {},
	"wechat": {},
	"x":      {},
	"zhihu":  {},
}

type Service struct {
	db              *gorm.DB
	router          *dbrouter.Router
	collabDocuments *collabdoc.Service
	cache           *redis.Client
	cacheTTL        time.Duration
	cacheGroup      *singleflight.Group
	statsCache      DashboardStatsCacheInvalidator
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
	if client == nil {
		return
	}
	s.cache = client
	s.cacheTTL = dashboardProjectListCacheTTL
}

func (s *Service) SetDashboardStatsCacheInvalidator(invalidator DashboardStatsCacheInvalidator) {
	s.statsCache = invalidator
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
