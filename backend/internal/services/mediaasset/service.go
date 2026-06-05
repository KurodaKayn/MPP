package mediaasset

import (
	"context"
	"errors"

	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	"gorm.io/gorm"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = projectsvc.ErrInvalidProject
var ErrMediaStorageUnavailable = errors.New("media storage unavailable")
var ErrInvalidMediaAsset = errors.New("invalid media asset")
var ErrMediaAssetUploadIncomplete = errors.New("media asset upload incomplete")
var ErrMediaAssetNotReady = errors.New("media asset not ready")

type Service struct {
	db            *gorm.DB
	projects      *projectsvc.Service
	objectStorage objectstorage.Client
	storageConfig objectstorage.Config
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	if projects == nil {
		projects = projectsvc.NewService(db)
	}
	return &Service{db: db, projects: projects}
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
