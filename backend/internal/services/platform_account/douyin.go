package platformaccount

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const douyinPlatform = "douyin"

func (s *Service) GetDouyinAccount(userID uuid.UUID) (*dto.DouyinAccountResponse, error) {
	return s.GetWorkspaceDouyinAccount(userID, uuid.Nil)
}

func (s *Service) GetWorkspaceDouyinAccount(userID uuid.UUID, workspaceID uuid.UUID) (*dto.DouyinAccountResponse, error) {
	var account models.PlatformAccount
	workspaceID = s.WorkspaceIDForUser(userID, workspaceID)
	err := s.strongReadDB().Where("workspace_id = ? AND platform = ?", workspaceID, douyinPlatform).Order("updated_at DESC").First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resp := emptyDouyinAccountResponse()
		return &resp, nil
	}
	if err != nil {
		return nil, err
	}

	resp := dto.DouyinAccountResponse{
		Platform:      douyinPlatform,
		Username:      account.Username,
		AvatarURL:     account.AvatarURL,
		Status:        normalizePlatformAccountStatus(account.Status),
		LastTestedAt:  account.LastTestedAt,
		LastTestError: account.LastTestError,
		UpdatedAt:     &account.UpdatedAt,
	}
	return &resp, nil
}

func emptyDouyinAccountResponse() dto.DouyinAccountResponse {
	return dto.DouyinAccountResponse{
		Platform: douyinPlatform,
		Status:   "unconfigured",
	}
}

func normalizePlatformAccountStatus(status string) string {
	if status == "" {
		return "unconfigured"
	}
	return status
}
