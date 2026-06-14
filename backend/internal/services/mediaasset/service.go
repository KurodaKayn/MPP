package mediaasset

import (
	"context"
	"errors"

	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = projectsvc.ErrInvalidProject
var ErrMediaStorageUnavailable = errors.New("media storage unavailable")
var ErrInvalidMediaAsset = errors.New("invalid media asset")
var ErrMediaAssetUploadIncomplete = errors.New("media asset upload incomplete")
var ErrMediaAssetNotReady = errors.New("media asset not ready")

type Service struct {
	db            *gorm.DB
	router        *dbrouter.Router
	projects      *projectsvc.Service
	objectStorage objectstorage.Client
	storageConfig objectstorage.Config
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	return NewServiceWithRouter(db, projects, nil)
}

func NewServiceWithRouter(db *gorm.DB, projects *projectsvc.Service, router *dbrouter.Router) *Service {
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	if projects == nil {
		projects = projectsvc.NewServiceWithRouter(db, router)
	}
	return &Service{db: db, router: router, projects: projects}
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
	return &scoped
}

func (s *Service) UseObjectStorage(client objectstorage.Client, config objectstorage.Config) {
	s.objectStorage = client
	s.storageConfig = config
}

func (s *Service) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

func (s *Service) writerDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Writer(s.requestContext())
}

func (s *Service) strongReadDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Reader(s.requestContext(), dbrouter.StrongRead)
}

func (s *Service) eventualReadDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Reader(s.requestContext(), dbrouter.EventualRead)
}
