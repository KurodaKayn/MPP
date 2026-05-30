package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const xPlatform = "x"

const (
	xAuthTypeOAuth1 = "oauth1"
	xAuthTypeOAuth2 = "oauth2"
)

type XConnectionTester interface {
	Test(ctx context.Context, credentials pkgx.Credentials) dto.XConnectionTestResponse
}

type XAPITester struct{}

type xCredentials struct {
	AuthType           string     `json:"auth_type,omitempty"`
	APIKey             string     `json:"api_key"`
	APISecret          string     `json:"api_secret"`
	AccessToken        string     `json:"access_token"`
	AccessTokenSecret  string     `json:"access_token_secret"`
	Username           string     `json:"username,omitempty"`
	OAuth2AccessToken  string     `json:"oauth2_access_token,omitempty"`
	OAuth2RefreshToken string     `json:"oauth2_refresh_token,omitempty"`
	OAuth2ExpiresAt    *time.Time `json:"oauth2_expires_at,omitempty"`
	OAuth2Scope        string     `json:"oauth2_scope,omitempty"`
}

type xMetadata struct {
	Name     string `json:"name,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

func (XAPITester) Test(ctx context.Context, credentials pkgx.Credentials) dto.XConnectionTestResponse {
	user, err := pkgx.NewClient(credentials).Me(ctx)
	return buildXConnectionResult(user, err)
}

func (s *DashboardService) GetXAccount(userID uuid.UUID) (*dto.XAccountResponse, error) {
	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, xPlatform).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resp := emptyXAccountResponse()
		return &resp, nil
	}
	if err != nil {
		return nil, err
	}

	resp, err := accountToXResponse(&account)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *DashboardService) UpsertXAccount(userID uuid.UUID, req dto.UpsertXAccountRequest) (*dto.XAccountResponse, error) {
	incoming := xCredentials{
		AuthType:          xAuthTypeOAuth1,
		APIKey:            strings.TrimSpace(req.APIKey),
		APISecret:         strings.TrimSpace(req.APISecret),
		AccessToken:       strings.TrimSpace(req.AccessToken),
		AccessTokenSecret: strings.TrimSpace(req.AccessTokenSecret),
		Username:          strings.Trim(strings.TrimSpace(req.Username), "@"),
	}

	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, xPlatform).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	existingCredentials, err := parseXCredentials(account.Credentials)
	if err != nil {
		return nil, err
	}
	merged := mergeXCredentials(existingCredentials, incoming)
	if err := merged.clientCredentials().Validate(); err != nil {
		return nil, fmt.Errorf("%w: api_key, api_secret, access_token and access_token_secret are required", ErrInvalidPlatformAccount)
	}

	credentials, err := marshalJSON(merged)
	if err != nil {
		return nil, err
	}

	if account.ID == uuid.Nil {
		account = models.PlatformAccount{
			UserID:      userID,
			Platform:    xPlatform,
			Name:        "X",
			Credentials: credentials,
			Metadata:    datatypes.JSON([]byte(`{}`)),
			Status:      models.PlatformAccountStatusUntested,
		}
		err = s.db.Create(&account).Error
	} else {
		err = s.db.Model(&account).Updates(map[string]interface{}{
			"name":            "X",
			"credentials":     credentials,
			"metadata":        datatypes.JSON([]byte(`{}`)),
			"status":          models.PlatformAccountStatusUntested,
			"last_tested_at":  nil,
			"last_test_error": "",
		}).Error
	}
	if err != nil {
		return nil, err
	}

	if err := s.db.Where("user_id = ? AND platform = ?", userID, xPlatform).First(&account).Error; err != nil {
		return nil, err
	}

	resp, err := accountToXResponse(&account)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *DashboardService) TestXAccount(userID uuid.UUID, req dto.TestXAccountRequest) (*dto.XConnectionTestResponse, error) {
	incoming := xCredentials{
		APIKey:            strings.TrimSpace(req.APIKey),
		APISecret:         strings.TrimSpace(req.APISecret),
		AccessToken:       strings.TrimSpace(req.AccessToken),
		AccessTokenSecret: strings.TrimSpace(req.AccessTokenSecret),
	}

	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, xPlatform).First(&account).Error
	accountExists := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	savedCredentials, err := parseXCredentials(account.Credentials)
	if err != nil {
		return nil, err
	}
	merged := mergeXCredentials(savedCredentials, incoming)
	if err := merged.clientCredentials().Validate(); err != nil {
		return nil, fmt.Errorf("%w: api_key, api_secret, access_token and access_token_secret are required", ErrInvalidPlatformAccount)
	}

	result := s.xTester.Test(context.Background(), merged.clientCredentials())

	if accountExists && xCredentialsMatch(savedCredentials, merged) {
		status := models.PlatformAccountStatusFailed
		errMessage := result.Message
		if result.Connected {
			status = models.PlatformAccountStatusConnected
			errMessage = ""
			if result.Username != "" {
				savedCredentials.Username = result.Username
			}
		}

		updates := map[string]interface{}{
			"status":          status,
			"last_tested_at":  result.TestedAt,
			"last_test_error": errMessage,
		}
		if result.Connected {
			credentials, err := marshalJSON(savedCredentials)
			if err != nil {
				return nil, err
			}
			metadata, err := marshalJSON(xMetadata{
				Name:     result.Name,
				UserID:   result.UserID,
				Username: result.Username,
			})
			if err != nil {
				return nil, err
			}
			updates["credentials"] = credentials
			updates["metadata"] = metadata
		}

		if err := s.db.Model(&account).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return &result, nil
}

func (s *DashboardService) applySavedXCredentialsToPublication(userID uuid.UUID, pub *models.ProjectPlatformPublication) error {
	if pub.Platform != xPlatform {
		return nil
	}

	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, xPlatform).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	credentials, err := parseXCredentials(account.Credentials)
	if err != nil {
		return err
	}

	config := map[string]interface{}{}
	if len(pub.Config) > 0 {
		if err := json.Unmarshal(pub.Config, &config); err != nil {
			return fmt.Errorf("failed to parse x publication config: %w", err)
		}
	}
	if config == nil {
		config = map[string]interface{}{}
	}

	switch credentials.authType() {
	case xAuthTypeOAuth2:
		credentials, err = s.refreshXOAuth2CredentialsIfNeeded(context.Background(), &account, credentials)
		if err != nil {
			return err
		}
		if strings.TrimSpace(credentials.OAuth2AccessToken) == "" {
			return nil
		}
		config["auth_type"] = xAuthTypeOAuth2
		config["oauth2_access_token"] = credentials.OAuth2AccessToken
		config["oauth2_refresh_token"] = credentials.OAuth2RefreshToken
		config["oauth2_scope"] = credentials.OAuth2Scope
		if credentials.OAuth2ExpiresAt != nil {
			config["oauth2_expires_at"] = credentials.OAuth2ExpiresAt
		}
		delete(config, "api_key")
		delete(config, "api_secret")
		delete(config, "access_token")
		delete(config, "access_token_secret")
	default:
		if err := credentials.clientCredentials().Validate(); err != nil {
			return nil
		}
		config["auth_type"] = xAuthTypeOAuth1
		config["api_key"] = credentials.APIKey
		config["api_secret"] = credentials.APISecret
		config["access_token"] = credentials.AccessToken
		config["access_token_secret"] = credentials.AccessTokenSecret
		delete(config, "oauth2_access_token")
		delete(config, "oauth2_refresh_token")
		delete(config, "oauth2_expires_at")
		delete(config, "oauth2_scope")
	}
	if credentials.Username != "" {
		config["username"] = credentials.Username
	}

	merged, err := marshalJSON(config)
	if err != nil {
		return err
	}
	pub.Config = merged
	return nil
}

func emptyXAccountResponse() dto.XAccountResponse {
	return dto.XAccountResponse{
		Platform:      xPlatform,
		AuthType:      xAuthTypeOAuth2,
		Status:        "unconfigured",
		AccountAuth:   unknownXAccountAuthHint(),
		PublishAccess: unknownXPublishAccessHint(),
	}
}

func accountToXResponse(account *models.PlatformAccount) (dto.XAccountResponse, error) {
	credentials, err := parseXCredentials(account.Credentials)
	if err != nil {
		return dto.XAccountResponse{}, err
	}

	metadata := xMetadata{}
	_ = json.Unmarshal(account.Metadata, &metadata)
	username := firstNonEmpty(metadata.Username, credentials.Username)

	updatedAt := account.UpdatedAt
	return dto.XAccountResponse{
		Platform:             account.Platform,
		AuthType:             credentials.authType(),
		APIKey:               credentials.APIKey,
		ExpiresAt:            credentials.OAuth2ExpiresAt,
		Username:             username,
		HasAPISecret:         credentials.APISecret != "",
		HasAccessToken:       credentials.AccessToken != "",
		HasAccessTokenSecret: credentials.AccessTokenSecret != "",
		HasOAuth2Refresh:     credentials.OAuth2RefreshToken != "",
		Status:               account.Status,
		LastTestedAt:         account.LastTestedAt,
		LastTestError:        account.LastTestError,
		UpdatedAt:            &updatedAt,
		AccountAuth:          xAccountAuthHintForStatus(account.Status, username),
		PublishAccess:        xPublishAccessHintForStatus(account.Status),
	}, nil
}

func parseXCredentials(raw datatypes.JSON) (xCredentials, error) {
	if len(raw) == 0 {
		return xCredentials{}, nil
	}

	var credentials xCredentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return xCredentials{}, err
	}
	return credentials, nil
}

func mergeXCredentials(existing, incoming xCredentials) xCredentials {
	merged := xCredentials{
		AuthType:          firstNonEmpty(incoming.AuthType, existing.AuthType),
		APIKey:            firstNonEmpty(incoming.APIKey, existing.APIKey),
		APISecret:         firstNonEmpty(incoming.APISecret, existing.APISecret),
		AccessToken:       firstNonEmpty(incoming.AccessToken, existing.AccessToken),
		AccessTokenSecret: firstNonEmpty(incoming.AccessTokenSecret, existing.AccessTokenSecret),
		Username:          firstNonEmpty(incoming.Username, existing.Username),
	}
	if incoming.AuthType != xAuthTypeOAuth1 {
		merged.OAuth2AccessToken = existing.OAuth2AccessToken
		merged.OAuth2RefreshToken = existing.OAuth2RefreshToken
		merged.OAuth2ExpiresAt = existing.OAuth2ExpiresAt
		merged.OAuth2Scope = existing.OAuth2Scope
	}
	if xCredentialIdentityChanged(existing, incoming) && incoming.Username == "" {
		merged.Username = ""
	}
	return merged
}

func xCredentialIdentityChanged(existing, incoming xCredentials) bool {
	return credentialFieldChanged(existing.APIKey, incoming.APIKey) ||
		credentialFieldChanged(existing.APISecret, incoming.APISecret) ||
		credentialFieldChanged(existing.AccessToken, incoming.AccessToken) ||
		credentialFieldChanged(existing.AccessTokenSecret, incoming.AccessTokenSecret)
}

func credentialFieldChanged(existing, incoming string) bool {
	return incoming != "" && incoming != existing
}

func xCredentialsMatch(left, right xCredentials) bool {
	return left.APIKey == right.APIKey &&
		left.APISecret == right.APISecret &&
		left.AccessToken == right.AccessToken &&
		left.AccessTokenSecret == right.AccessTokenSecret
}

func (c xCredentials) clientCredentials() pkgx.Credentials {
	return pkgx.Credentials{
		APIKey:            c.APIKey,
		APISecret:         c.APISecret,
		AccessToken:       c.AccessToken,
		AccessTokenSecret: c.AccessTokenSecret,
	}
}

func (c xCredentials) authType() string {
	switch c.AuthType {
	case xAuthTypeOAuth1, xAuthTypeOAuth2:
		return c.AuthType
	default:
		if c.OAuth2AccessToken != "" {
			return xAuthTypeOAuth2
		}
		if c.APIKey != "" || c.AccessToken != "" {
			return xAuthTypeOAuth1
		}
		return xAuthTypeOAuth2
	}
}

func buildXConnectionResult(user pkgx.User, err error) dto.XConnectionTestResponse {
	testedAt := time.Now()
	if err == nil {
		return dto.XConnectionTestResponse{
			Connected:     true,
			Status:        models.PlatformAccountStatusConnected,
			Message:       "连接成功，X 已接受当前 OAuth 1.0a 用户凭证。",
			TestedAt:      testedAt,
			UserID:        user.ID,
			Username:      user.Username,
			Name:          user.Name,
			AccountAuth:   connectedXAccountAuthHint(user.Username),
			PublishAccess: connectedXPublishAccessHint(),
		}
	}

	return dto.XConnectionTestResponse{
		Connected:     false,
		Status:        models.PlatformAccountStatusFailed,
		Message:       "连接失败：无法通过 X 用户凭证校验，请确认 App 权限和 Access Token。",
		TestedAt:      testedAt,
		AccountAuth:   failedXAccountAuthHint(),
		PublishAccess: unknownXPublishAccessHint(),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func xAccountAuthHintForStatus(status, username string) dto.RequirementStatus {
	switch status {
	case models.PlatformAccountStatusConnected:
		return connectedXAccountAuthHint(username)
	case models.PlatformAccountStatusFailed:
		return failedXAccountAuthHint()
	default:
		return unknownXAccountAuthHint()
	}
}

func xPublishAccessHintForStatus(status string) dto.RequirementStatus {
	switch status {
	case models.PlatformAccountStatusConnected:
		return connectedXPublishAccessHint()
	case models.PlatformAccountStatusFailed:
		return unknownXPublishAccessHint()
	default:
		return unknownXPublishAccessHint()
	}
}

func connectedXAccountAuthHint(username string) dto.RequirementStatus {
	message := "OAuth 1.0a 用户凭证已通过 X API 校验。"
	if username != "" {
		message = "已连接 @" + username + "。"
	}
	return dto.RequirementStatus{
		Status:  "passed",
		Title:   "账号凭证已通过",
		Message: message,
	}
}

func connectedXPublishAccessHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "warning",
		Title:   "需确认写入权限",
		Message: "测试会校验账号身份；实际发布还要求 X App 具备 Read and write 权限。",
	}
}

func failedXAccountAuthHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "failed",
		Title:   "账号凭证未通过",
		Message: "请确认 API Key、API Secret、Access Token、Access Token Secret 均来自同一个 X App 和账号。",
	}
}

func unknownXAccountAuthHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "unknown",
		Title:   "等待测试",
		Message: "保存 X OAuth 1.0a 用户凭证后可执行连接测试。",
	}
}

func unknownXPublishAccessHint() dto.RequirementStatus {
	return dto.RequirementStatus{
		Status:  "unknown",
		Title:   "等待测试",
		Message: "发布前请确认 X App 开启 Read and write 用户权限。",
	}
}
