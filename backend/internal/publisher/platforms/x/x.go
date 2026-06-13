package x

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher/core"
)

const xPostIntentURL = "https://x.com/intent/tweet"

const (
	xAuthTypeOAuth1 = "oauth1"
	xAuthTypeOAuth2 = "oauth2"
)

type xTweetClient interface {
	CreateTweet(ctx context.Context, text string) (pkgx.Tweet, error)
}

var (
	newXOAuth1TweetClient = func(creds pkgx.Credentials) xTweetClient {
		return pkgx.NewClient(creds)
	}
	newXOAuth2TweetClient = func(creds pkgx.OAuth2Credentials) xTweetClient {
		return pkgx.NewOAuth2Client(creds)
	}
)

type XPublisher struct{}

type XConfig struct {
	AuthType           string     `json:"auth_type"`
	APIKey             string     `json:"api_key"`
	APISecret          string     `json:"api_secret"`
	AccessToken        string     `json:"access_token"`
	AccessTokenSecret  string     `json:"access_token_secret"`
	Username           string     `json:"username"`
	OAuth2AccessToken  string     `json:"oauth2_access_token"`
	OAuth2RefreshToken string     `json:"oauth2_refresh_token"`
	OAuth2ExpiresAt    *time.Time `json:"oauth2_expires_at"`
	OAuth2Scope        string     `json:"oauth2_scope"`
}

func (x *XPublisher) ValidateConfig(config []byte) error {
	var cfg XConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return err
	}
	return cfg.validate()
}

func BuildXPostIntentURL(raw []byte) (string, error) {
	text := extractXText(raw)
	if text == "" {
		return "", fmt.Errorf("x post text is empty")
	}

	endpoint, err := url.Parse(xPostIntentURL)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("text", text)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (x *XPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	var cfg XConfig
	if err := json.Unmarshal(pub.Config, &cfg); err != nil {
		return "", "", fmt.Errorf("failed to parse x config: %w", err)
	}
	cfg, err := cfg.withAccountCredentials(account)
	if err != nil {
		return "", "", err
	}
	if err := cfg.validate(); err != nil {
		return "", "", err
	}

	text := extractXText(pub.AdaptedContent)
	if text == "" {
		return "", "", fmt.Errorf("x post text is empty")
	}

	tweet, err := cfg.tweetClient().CreateTweet(ctx, text)
	if err != nil {
		return "", "", err
	}

	return tweet.ID, xStatusURL(cfg.Username, tweet.ID), nil
}

func (c XConfig) validate() error {
	if c.authType() == xAuthTypeOAuth2 {
		return c.oauth2Credentials().Validate()
	}
	return c.credentials().Validate()
}

func (c XConfig) authType() string {
	switch strings.TrimSpace(c.AuthType) {
	case xAuthTypeOAuth2:
		return xAuthTypeOAuth2
	case xAuthTypeOAuth1:
		return xAuthTypeOAuth1
	default:
		if strings.TrimSpace(c.OAuth2AccessToken) != "" {
			return xAuthTypeOAuth2
		}
		return xAuthTypeOAuth1
	}
}

func (c XConfig) tweetClient() xTweetClient {
	if c.authType() == xAuthTypeOAuth2 {
		return newXOAuth2TweetClient(c.oauth2Credentials())
	}
	return newXOAuth1TweetClient(c.credentials())
}

func (c XConfig) credentials() pkgx.Credentials {
	return pkgx.Credentials{
		APIKey:            strings.TrimSpace(c.APIKey),
		APISecret:         strings.TrimSpace(c.APISecret),
		AccessToken:       strings.TrimSpace(c.AccessToken),
		AccessTokenSecret: strings.TrimSpace(c.AccessTokenSecret),
	}
}

func (c XConfig) oauth2Credentials() pkgx.OAuth2Credentials {
	return pkgx.OAuth2Credentials{
		AccessToken: strings.TrimSpace(c.OAuth2AccessToken),
	}
}

func (c XConfig) withAccountCredentials(account *models.PlatformAccount) (XConfig, error) {
	if account == nil || len(account.Credentials) == 0 {
		return c, nil
	}

	var saved XConfig
	if err := json.Unmarshal(account.Credentials, &saved); err != nil {
		return XConfig{}, fmt.Errorf("failed to parse x account credentials: %w", err)
	}
	switch saved.authType() {
	case xAuthTypeOAuth2:
		c.AuthType = xAuthTypeOAuth2
		c.OAuth2AccessToken = firstNonEmptyXString(saved.OAuth2AccessToken, c.OAuth2AccessToken)
		c.OAuth2RefreshToken = firstNonEmptyXString(saved.OAuth2RefreshToken, c.OAuth2RefreshToken)
		c.OAuth2Scope = firstNonEmptyXString(saved.OAuth2Scope, c.OAuth2Scope)
		c.OAuth2ExpiresAt = firstNonNilTime(saved.OAuth2ExpiresAt, c.OAuth2ExpiresAt)
	default:
		c.AuthType = xAuthTypeOAuth1
		c.APIKey = firstNonEmptyXString(saved.APIKey, c.APIKey)
		c.APISecret = firstNonEmptyXString(saved.APISecret, c.APISecret)
		c.AccessToken = firstNonEmptyXString(saved.AccessToken, c.AccessToken)
		c.AccessTokenSecret = firstNonEmptyXString(saved.AccessTokenSecret, c.AccessTokenSecret)
	}
	c.Username = firstNonEmptyXString(saved.Username, c.Username)
	return c, nil
}

func firstNonEmptyXString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonNilTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func extractXText(raw []byte) string {
	var structured core.AdaptedContent
	if err := json.Unmarshal(raw, &structured); err == nil {
		if structured.Text != nil {
			if text := strings.TrimSpace(*structured.Text); text != "" {
				return text
			}
		}
		if structured.Summary != nil {
			if summary := strings.TrimSpace(*structured.Summary); summary != "" {
				return summary
			}
		}
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}

	return strings.TrimSpace(string(raw))
}

func xStatusURL(username, tweetID string) string {
	username = strings.Trim(strings.TrimSpace(username), "@")
	if username == "" {
		return "https://x.com/i/web/status/" + tweetID
	}
	return "https://x.com/" + username + "/status/" + tweetID
}
