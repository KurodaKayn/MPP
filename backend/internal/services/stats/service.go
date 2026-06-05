package stats

import (
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	projects *projectsvc.Service
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	return &Service{db: db, projects: projects}
}
