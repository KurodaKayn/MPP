package stats

import (
	"gorm.io/gorm"

	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

type Service struct {
	db       *gorm.DB
	projects *projectsvc.Service
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	return &Service{db: db, projects: projects}
}
