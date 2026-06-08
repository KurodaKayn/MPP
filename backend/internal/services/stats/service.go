package stats

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

const dashboardStatsCacheTTL = 15 * time.Second

type Service struct {
	db         *gorm.DB
	router     *dbrouter.Router
	projects   *projectsvc.Service
	cache      *redis.Client
	cacheTTL   time.Duration
	cacheGroup *singleflight.Group
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	return NewServiceWithRouter(db, projects, nil)
}

func NewServiceWithRouter(db *gorm.DB, projects *projectsvc.Service, router *dbrouter.Router) *Service {
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	return &Service{
		db:         db,
		router:     router,
		projects:   projects,
		cacheGroup: &singleflight.Group{},
	}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	if s.projects != nil {
		scoped.projects = s.projects.WithContext(ctx)
	}
	scoped.cacheGroup = s.cacheGroup
	return &scoped
}

func (s *Service) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.cache = client
	s.cacheTTL = dashboardStatsCacheTTL
	if s.cacheGroup == nil {
		s.cacheGroup = &singleflight.Group{}
	}
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

func (s *Service) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}
