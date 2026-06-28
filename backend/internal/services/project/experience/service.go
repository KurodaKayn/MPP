package experience

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	"github.com/kurodakayn/mpp-backend/internal/services/project/projecterr"
)

var ErrInvalidProjectComment = projecterr.ErrInvalidProjectComment
var ErrInvalidProjectShareLink = projecterr.ErrInvalidProjectShareLink
var ErrInvalidProjectVersion = projecterr.ErrInvalidProjectVersion

type ProjectGetter func(uuid.UUID, *uuid.UUID) (*dto.ProjectDetail, error)
type ProjectMediaUsageRefresher func(*gorm.DB, models.Project, []models.ProjectPlatformPublication) error
type DashboardCachesInvalidator func(includeStats bool)
type ScopedStatsInvalidator func()

type Service struct {
	db                         *gorm.DB
	getProject                 ProjectGetter
	refreshProjectMediaUsages  ProjectMediaUsageRefresher
	invalidateDashboardCaches  DashboardCachesInvalidator
	invalidateScopedStatsCache ScopedStatsInvalidator
}

func NewService(
	db *gorm.DB,
	getProject ProjectGetter,
	refreshProjectMediaUsages ProjectMediaUsageRefresher,
	invalidateDashboardCaches DashboardCachesInvalidator,
	invalidateScopedStatsCache ScopedStatsInvalidator,
) *Service {
	return &Service{
		db:                         db,
		getProject:                 getProject,
		refreshProjectMediaUsages:  refreshProjectMediaUsages,
		invalidateDashboardCaches:  invalidateDashboardCaches,
		invalidateScopedStatsCache: invalidateScopedStatsCache,
	}
}

func (s *Service) accessibleProject(projectID uuid.UUID, userID uuid.UUID) (models.Project, error) {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return models.Project{}, projecterr.ErrInvalidProject
	}
	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return models.Project{}, err
	}
	_, err := accesspolicy.ProjectAccessRoleWithDB(s.db, project, userID)
	return project, err
}

func selectUserIdentity(db *gorm.DB) *gorm.DB {
	return db.Select("id", "username", "email")
}

func JSONMap(value map[string]any) (datatypes.JSON, error) {
	if value == nil {
		return datatypes.JSON([]byte(`{}`)), nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func MapFromJSON(value datatypes.JSON) map[string]any {
	var parsed map[string]any
	if err := json.Unmarshal(value, &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func uuidString(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}
