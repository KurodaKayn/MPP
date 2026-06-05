package project

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = errors.New("invalid project")
var ErrInvalidProjectCollaborator = errors.New("invalid project collaborator")
var ErrProjectCollabUnavailable = errors.New("project collaboration unavailable")

var allowedProjectPlatforms = map[string]struct{}{
	"douyin": {},
	"wechat": {},
	"x":      {},
	"zhihu":  {},
}

type Service struct {
	db              *gorm.DB
	collabDocuments *collabdoc.Service
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	if s.collabDocuments != nil {
		scoped.collabDocuments = s.collabDocuments.WithContext(ctx)
	}
	return &scoped
}

func (s *Service) SetCollabDocumentService(svc *collabdoc.Service) {
	s.collabDocuments = svc
}

func (s *Service) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

func ensurePersonalWorkspace(tx *gorm.DB, ownerUserID uuid.UUID) error {
	workspaceID := models.PersonalWorkspaceID(ownerUserID)
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerUserID,
		Name:        models.PersonalWorkspaceName,
		Slug:        models.PersonalWorkspaceSlug(ownerUserID),
		Status:      models.WorkspaceStatusActive,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&workspace).Error; err != nil {
		return err
	}

	now := time.Now()
	member := models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      ownerUserID,
		Role:        models.WorkspaceRoleOwner,
		JoinedAt:    &now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(&member).Error
}
