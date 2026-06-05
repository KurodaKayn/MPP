package workspace

import (
	"errors"

	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	"gorm.io/gorm"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidWorkspace = errors.New("invalid workspace")
var ErrInvalidWorkspaceMember = errors.New("invalid workspace member")

type Service struct {
	db       *gorm.DB
	projects *projectsvc.Service
}

func NewService(db *gorm.DB, projects *projectsvc.Service) *Service {
	return &Service{db: db, projects: projects}
}
