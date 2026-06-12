package browsersession

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
)

func (s *BrowserSessionService) authorizeSessionTarget(ctx context.Context, userID uuid.UUID, workspaceID uuid.UUID, accountID uuid.UUID, platform string) (uuid.UUID, error) {
	if workspaceID == uuid.Nil {
		workspaceID = models.PersonalWorkspaceID(userID)
	}
	if accountID != uuid.Nil {
		return s.authorizeSessionAccount(ctx, userID, workspaceID, accountID, platform)
	}
	if err := s.requireWorkspaceAccountConnect(ctx, userID, workspaceID); err != nil {
		return uuid.Nil, err
	}
	return workspaceID, nil
}

func (s *BrowserSessionService) authorizeSessionAccount(ctx context.Context, userID uuid.UUID, workspaceID uuid.UUID, accountID uuid.UUID, platform string) (uuid.UUID, error) {
	db := s.dbWithContext(ctx)
	var account models.PlatformAccount
	err := db.
		Where("id = ? AND workspace_id = ? AND platform = ?", accountID, workspaceID, platform).
		First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, ErrPlatformAccountForbidden
		}
		return uuid.Nil, err
	}

	if err := platformaccount.NewService(db).RequireAccountUse(userID, account, uuid.Nil); err != nil {
		if errors.Is(err, platformaccount.ErrPlatformAccountForbidden) {
			return uuid.Nil, ErrPlatformAccountForbidden
		}
		return uuid.Nil, err
	}
	if account.WorkspaceID == nil {
		return uuid.Nil, ErrPlatformAccountForbidden
	}
	return *account.WorkspaceID, nil
}

func (s *BrowserSessionService) requireWorkspaceAccountConnect(ctx context.Context, userID uuid.UUID, workspaceID uuid.UUID) error {
	if workspaceID == models.PersonalWorkspaceID(userID) {
		return nil
	}

	db := s.dbWithContext(ctx)
	var workspace models.Workspace
	if err := db.Select("owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformAccountForbidden
		}
		return err
	}
	if workspace.OwnerUserID == userID {
		return nil
	}

	var member models.WorkspaceMember
	if err := db.Select("role").First(&member, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformAccountForbidden
		}
		return err
	}
	switch member.Role {
	case models.WorkspaceRoleOwner, models.WorkspaceRoleAdmin:
		return nil
	default:
		return ErrPlatformAccountForbidden
	}
}

func (s *BrowserSessionService) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return s.db
	}
	return s.db.WithContext(ctx)
}
