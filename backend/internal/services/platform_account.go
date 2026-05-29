package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/dto"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/pkg/wechat"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const wechatPlatform = "wechat"

var ErrInvalidPlatformAccount = errors.New("invalid platform account settings")

type WechatConnectionTester interface {
	Test(appID, appSecret string) dto.WechatConnectionTestResponse
}

type WechatAPITester struct{}

type wechatCredentials struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

func (WechatAPITester) Test(appID, appSecret string) dto.WechatConnectionTestResponse {
	_, err := wechat.NewClient(appID, appSecret).GetToken()
	return buildWechatConnectionResult(err)
}

func (s *DashboardService) GetWechatAccount(userID uuid.UUID) (*dto.WechatAccountResponse, error) {
	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, wechatPlatform).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resp := emptyWechatAccountResponse()
		return &resp, nil
	}
	if err != nil {
		return nil, err
	}

	resp, err := accountToWechatResponse(&account)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *DashboardService) UpsertWechatAccount(userID uuid.UUID, req dto.UpsertWechatAccountRequest) (*dto.WechatAccountResponse, error) {
	appID := strings.TrimSpace(req.AppID)
	appSecret := strings.TrimSpace(req.AppSecret)
	if appID == "" {
		return nil, fmt.Errorf("%w: app_id is required", ErrInvalidPlatformAccount)
	}

	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, wechatPlatform).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	existingCredentials, err := parseWechatCredentials(account.Credentials)
	if err != nil {
		return nil, err
	}
	if appSecret == "" {
		appSecret = existingCredentials.AppSecret
	}
	if appSecret == "" {
		return nil, fmt.Errorf("%w: app_secret is required", ErrInvalidPlatformAccount)
	}

	credentials, err := marshalJSON(wechatCredentials{
		AppID:     appID,
		AppSecret: appSecret,
	})
	if err != nil {
		return nil, err
	}

	if account.ID == uuid.Nil {
		account = models.PlatformAccount{
			UserID:      userID,
			Platform:    wechatPlatform,
			Name:        "微信公众号",
			Credentials: credentials,
			Metadata:    datatypes.JSON([]byte(`{}`)),
			Status:      models.PlatformAccountStatusUntested,
		}
		err = s.db.Create(&account).Error
	} else {
		err = s.db.Model(&account).Updates(map[string]interface{}{
			"name":            "微信公众号",
			"credentials":     credentials,
			"status":          models.PlatformAccountStatusUntested,
			"last_tested_at":  nil,
			"last_test_error": "",
		}).Error
	}
	if err != nil {
		return nil, err
	}

	if err := s.db.Where("user_id = ? AND platform = ?", userID, wechatPlatform).First(&account).Error; err != nil {
		return nil, err
	}

	resp, err := accountToWechatResponse(&account)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *DashboardService) TestWechatAccount(userID uuid.UUID, req dto.TestWechatAccountRequest) (*dto.WechatConnectionTestResponse, error) {
	appID := strings.TrimSpace(req.AppID)
	appSecret := strings.TrimSpace(req.AppSecret)

	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, wechatPlatform).First(&account).Error
	accountExists := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if accountExists {
		credentials, err := parseWechatCredentials(account.Credentials)
		if err != nil {
			return nil, err
		}
		if appID == "" {
			appID = credentials.AppID
		}
		if appSecret == "" {
			appSecret = credentials.AppSecret
		}
	}

	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("%w: app_id and app_secret are required", ErrInvalidPlatformAccount)
	}

	result := s.wechatTester.Test(appID, appSecret)

	if accountExists {
		status := models.PlatformAccountStatusFailed
		errMessage := result.Message
		if result.Connected {
			status = models.PlatformAccountStatusConnected
			errMessage = ""
		}

		if err := s.db.Model(&account).Updates(map[string]interface{}{
			"status":          status,
			"last_tested_at":  result.TestedAt,
			"last_test_error": errMessage,
		}).Error; err != nil {
			return nil, err
		}
	}

	return &result, nil
}

func emptyWechatAccountResponse() dto.WechatAccountResponse {
	return dto.WechatAccountResponse{
		Platform:    wechatPlatform,
		Status:      "unconfigured",
		IPWhitelist: unknownIPWhitelistHint(),
		AccountAuth: unknownAccountAuthHint(),
	}
}

func accountToWechatResponse(account *models.PlatformAccount) (dto.WechatAccountResponse, error) {
	credentials, err := parseWechatCredentials(account.Credentials)
	if err != nil {
		return dto.WechatAccountResponse{}, err
	}

	updatedAt := account.UpdatedAt
	return dto.WechatAccountResponse{
		Platform:      account.Platform,
		AppID:         credentials.AppID,
		HasAppSecret:  credentials.AppSecret != "",
		Status:        account.Status,
		LastTestedAt:  account.LastTestedAt,
		LastTestError: account.LastTestError,
		UpdatedAt:     &updatedAt,
		IPWhitelist:   ipWhitelistHintForStatus(account.Status),
		AccountAuth:   accountAuthHintForStatus(account.Status),
	}, nil
}

func parseWechatCredentials(raw datatypes.JSON) (wechatCredentials, error) {
	if len(raw) == 0 {
		return wechatCredentials{}, nil
	}

	var credentials wechatCredentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return wechatCredentials{}, err
	}
	return credentials, nil
}

func marshalJSON(value interface{}) (datatypes.JSON, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(body), nil
}

func buildWechatConnectionResult(err error) dto.WechatConnectionTestResponse {
	testedAt := time.Now()
	if err == nil {
		return dto.WechatConnectionTestResponse{
			Connected:   true,
			Status:      models.PlatformAccountStatusConnected,
			Message:     "连接成功，AppID/AppSecret 和当前服务器出口 IP 已通过微信接口校验。",
			TestedAt:    testedAt,
			IPWhitelist: passedIPWhitelistHint(),
			AccountAuth: connectedAccountAuthHint(),
		}
	}

	resp := dto.WechatConnectionTestResponse{
		Connected:   false,
		Status:      models.PlatformAccountStatusFailed,
		Message:     "连接失败：" + err.Error(),
		TestedAt:    testedAt,
		IPWhitelist: unknownIPWhitelistHint(),
		AccountAuth: unknownAccountAuthHint(),
	}

	var apiErr wechat.APIError
	if errors.As(err, &apiErr) {
		resp.ErrCode = apiErr.ErrCode
		resp.ErrMsg = apiErr.ErrMsg
		switch apiErr.ErrCode {
		case 40164:
			resp.Message = "连接失败：当前服务器出口 IP 未加入公众号后台 IP 白名单。"
			resp.IPWhitelist = dto.RequirementStatus{
				Status:  "failed",
				Title:   "IP 白名单未通过",
				Message: "请到微信公众平台的开发设置里，把后端服务器出口 IP 加入 IP 白名单后重试。",
			}
		case 40013:
			resp.Message = "连接失败：AppID 无效。请确认填写的是微信公众号 AppID，不是原始 ID、小程序 AppID 或开放平台 AppID。"
		case 40125:
			resp.Message = "连接失败：AppSecret 无效。请确认 AppSecret 与当前微信公众号 AppID 属于同一账号。"
		}
	}

	return resp
}

func ipWhitelistHintForStatus(status string) dto.RequirementStatus {
	switch status {
	case models.PlatformAccountStatusConnected:
		return passedIPWhitelistHint()
	case models.PlatformAccountStatusFailed:
		return unknownIPWhitelistHint()
	default:
		return unknownIPWhitelistHint()
	}
}

func accountAuthHintForStatus(status string) dto.RequirementStatus {
	switch status {
	case models.PlatformAccountStatusConnected:
		return connectedAccountAuthHint()
	default:
		return unknownAccountAuthHint()
	}
}

func passedIPWhitelistHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "passed",
		Title:   "IP 白名单已通过",
		Message: "微信 access_token 接口已接受当前服务器请求。",
	}
}

func unknownIPWhitelistHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "unknown",
		Title:   "等待测试",
		Message: "测试连接后会根据微信返回结果判断当前服务器出口 IP 是否已加入白名单。",
	}
}

func connectedAccountAuthHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "warning",
		Title:   "需确认认证与发布权限",
		Message: "连接成功不等于已具备发布权限；公众号仍需在微信公众平台完成认证，并具备草稿/发布相关接口权限。",
	}
}

func unknownAccountAuthHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "unknown",
		Title:   "无法自动确认",
		Message: "微信 token 接口不直接返回公众号认证状态；完成连接测试后，仍需在公众平台确认账号认证和接口权限。",
	}
}
