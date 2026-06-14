package browsersession

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
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
	if err := accesspolicy.RequireWorkspaceAccountConnectWithDB(s.dbWithContext(ctx), workspaceID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) || errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformAccountForbidden
		}
		return err
	}
	return nil
}

func (s *BrowserSessionService) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return s.db
	}
	return s.db.WithContext(ctx)
}
