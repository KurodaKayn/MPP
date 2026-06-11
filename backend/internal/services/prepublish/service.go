package prepublish

import (
	"context"

	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/compiler"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = projectsvc.ErrInvalidProject

type ProjectDraftCompiler interface {
	CompileProjectDrafts(ctx context.Context, project *models.Project, publications []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error)
}

type DashboardStatsCacheInvalidator interface {
	InvalidateDashboardStatsCache(ctx context.Context)
}

type Service struct {
	db         *gorm.DB
	projects   *projectsvc.Service
	statsCache DashboardStatsCacheInvalidator

	draftCompiler ProjectDraftCompiler
}

func NewService(db *gorm.DB, projects *projectsvc.Service, draftCompiler ProjectDraftCompiler) *Service {
	return &Service{db: db, projects: projects, draftCompiler: draftCompiler}
}

func (s *Service) SetDraftCompiler(draftCompiler ProjectDraftCompiler) {
	s.draftCompiler = draftCompiler
}

func (s *Service) SetDashboardStatsCacheInvalidator(invalidator DashboardStatsCacheInvalidator) {
	s.statsCache = invalidator
}

func (s *Service) DraftCompiler() ProjectDraftCompiler {
	if s == nil {
		return nil
	}
	return s.draftCompiler
}

func (s *Service) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

func (s *Service) invalidateDashboardCaches() {
	ctx := s.requestContext()
	if s.projects != nil {
		s.projects.InvalidateDashboardProjectListCache(ctx)
	}
	if s.statsCache != nil {
		s.statsCache.InvalidateDashboardStatsCache(ctx)
	}
}

func defaultDraftCompiler() ProjectDraftCompiler {
	return compiler.NewContentPipelineDraftCompiler()
}
