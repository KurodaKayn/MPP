package dto

import "time"

type RequirementStatus struct {
	Status  string `json:"status"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

type WechatAccountResponse struct {
	Platform      string            `json:"platform"`
	AppID         string            `json:"app_id"`
	HasAppSecret  bool              `json:"has_app_secret"`
	Status        string            `json:"status"`
	LastTestedAt  *time.Time        `json:"last_tested_at,omitempty"`
	LastTestError string            `json:"last_test_error,omitempty"`
	UpdatedAt     *time.Time        `json:"updated_at,omitempty"`
	IPWhitelist   RequirementStatus `json:"ip_whitelist"`
	AccountAuth   RequirementStatus `json:"account_auth"`
}

type UpsertWechatAccountRequest struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type TestWechatAccountRequest struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type WechatConnectionTestResponse struct {
	Connected   bool              `json:"connected"`
	Status      string            `json:"status"`
	Message     string            `json:"message"`
	ErrCode     int               `json:"err_code,omitempty"`
	ErrMsg      string            `json:"err_msg,omitempty"`
	TestedAt    time.Time         `json:"tested_at"`
	IPWhitelist RequirementStatus `json:"ip_whitelist"`
	AccountAuth RequirementStatus `json:"account_auth"`
}

type XAccountResponse struct {
	Platform             string            `json:"platform"`
	AuthType             string            `json:"auth_type"`
	APIKey               string            `json:"api_key,omitempty"`
	ExpiresAt            *time.Time        `json:"expires_at,omitempty"`
	Username             string            `json:"username,omitempty"`
	HasAPISecret         bool              `json:"has_api_secret"`
	HasAccessToken       bool              `json:"has_access_token"`
	HasAccessTokenSecret bool              `json:"has_access_token_secret"`
	HasOAuth2Refresh     bool              `json:"has_oauth2_refresh"`
	Status               string            `json:"status"`
	LastTestedAt         *time.Time        `json:"last_tested_at,omitempty"`
	LastTestError        string            `json:"last_test_error,omitempty"`
	UpdatedAt            *time.Time        `json:"updated_at,omitempty"`
	AccountAuth          RequirementStatus `json:"account_auth"`
	PublishAccess        RequirementStatus `json:"publish_access"`
}

type UpsertXAccountRequest struct {
	APIKey            string `json:"api_key"`
	APISecret         string `json:"api_secret"`
	AccessToken       string `json:"access_token"`
	AccessTokenSecret string `json:"access_token_secret"`
	Username          string `json:"username"`
}

type TestXAccountRequest struct {
	APIKey            string `json:"api_key"`
	APISecret         string `json:"api_secret"`
	AccessToken       string `json:"access_token"`
	AccessTokenSecret string `json:"access_token_secret"`
}

type XConnectionTestResponse struct {
	Connected     bool              `json:"connected"`
	Status        string            `json:"status"`
	Message       string            `json:"message"`
	TestedAt      time.Time         `json:"tested_at"`
	UserID        string            `json:"user_id,omitempty"`
	Username      string            `json:"username,omitempty"`
	Name          string            `json:"name,omitempty"`
	AccountAuth   RequirementStatus `json:"account_auth"`
	PublishAccess RequirementStatus `json:"publish_access"`
}

type DouyinAccountResponse struct {
	Platform      string     `json:"platform"`
	Username      string     `json:"username,omitempty"`
	AvatarURL     string     `json:"avatar_url,omitempty"`
	Status        string     `json:"status"`
	LastTestedAt  *time.Time `json:"last_tested_at,omitempty"`
	LastTestError string     `json:"last_test_error,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}
