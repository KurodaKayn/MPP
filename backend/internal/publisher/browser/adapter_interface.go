package browser

import (
	"context"
)

type RemoteLoginState struct {
	LoggedIn       bool     `json:"logged_in"`
	Status         string   `json:"status"`
	CurrentURL     string   `json:"current_url"`
	MissingCookies []string `json:"missing_cookies"`
	Message        string   `json:"message"`
}

type RemoteBrowserPlatformAdapter interface {
	Platform() string
	LoginURL() string
	AllowedDomains() []DomainRule
	RequiredCookies() []CookieRequirement
	DetectLogin(ctx context.Context) (RemoteLoginState, error)
	ExtractAccount(ctx context.Context) (RemoteAccountProfile, error)
}
