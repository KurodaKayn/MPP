package stats

import (
	"context"

	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

type Service struct {
	db       *gorm.DB
	router   *dbrouter.Router
	projects *projectsvc.Service
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
