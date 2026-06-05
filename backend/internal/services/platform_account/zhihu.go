package platformaccount

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const zhihuPlatform = "zhihu"

func (s *Service) GetZhihuAccount(userID uuid.UUID) (*dto.ZhihuAccountResponse, error) {
	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, zhihuPlatform).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resp := emptyZhihuAccountResponse()
		return &resp, nil
	}
	if err != nil {
		return nil, err
	}

	resp := dto.ZhihuAccountResponse{
		Platform:      zhihuPlatform,
		Username:      account.Username,
		AvatarURL:     account.AvatarURL,
		Status:        normalizePlatformAccountStatus(account.Status),
		LastTestedAt:  account.LastTestedAt,
		LastTestError: account.LastTestError,
		UpdatedAt:     &account.UpdatedAt,
	}
	return &resp, nil
}

func emptyZhihuAccountResponse() dto.ZhihuAccountResponse {
	return dto.ZhihuAccountResponse{
		Platform: zhihuPlatform,
		Status:   "unconfigured",
	}
}
