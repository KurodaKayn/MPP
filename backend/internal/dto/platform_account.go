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
