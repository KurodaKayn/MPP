package extension

import (
	"context"
	"errors"

	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = projectsvc.ErrInvalidProject
var ErrPublicationDisabled = publishsvc.ErrPublicationDisabled
var ErrPublicationRequiresSync = publishsvc.ErrPublicationRequiresSync
var ErrExtensionCallbackTokenInvalid = errors.New("invalid extension callback token")
var ErrExtensionCallbackTokenExpired = errors.New("expired extension callback token")

type Service struct {
	db     *gorm.DB
	router *dbrouter.Router
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithRouter(db, nil)
}

func NewServiceWithRouter(db *gorm.DB, router *dbrouter.Router) *Service {
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	return &Service{db: db, router: router}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	return &scoped
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
