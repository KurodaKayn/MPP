package platformaccount

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	xOAuth2ClientIDEnv     = "X_OAUTH2_CLIENT_ID"
	xOAuth2ClientSecretEnv = "X_OAUTH2_CLIENT_SECRET"
	xOAuth2AuthorizeURLEnv = "X_OAUTH2_AUTHORIZE_URL"
	xOAuth2TokenURLEnv     = "X_OAUTH2_TOKEN_URL"
	xOAuth2RefreshSkew     = 5 * time.Minute
	xOAuth2StateTTL        = 10 * time.Minute
)

var (
	ErrXOAuth2NotConfigured = errors.New("x oauth2 is not configured")
	ErrInvalidXOAuth2State  = errors.New("invalid x oauth2 state")
)

type XOAuth2Provider interface {
	AuthorizationURL(config pkgx.OAuth2Config, state, codeChallenge string) (string, error)
	Exchange(ctx context.Context, config pkgx.OAuth2Config, code, codeVerifier string) (pkgx.OAuth2Token, error)
	Refresh(ctx context.Context, config pkgx.OAuth2Config, refreshToken string) (pkgx.OAuth2Token, error)
	Me(ctx context.Context, accessToken string) (pkgx.User, error)
}

type XOAuth2API struct{}

type xOAuth2PendingState struct {
	UserID       uuid.UUID
	CodeVerifier string
	RedirectURI  string
	ExpiresAt    time.Time
}

func (XOAuth2API) AuthorizationURL(config pkgx.OAuth2Config, state, codeChallenge string) (string, error) {
	return config.AuthorizationURL(state, codeChallenge)
}

func (XOAuth2API) Exchange(ctx context.Context, config pkgx.OAuth2Config, code, codeVerifier string) (pkgx.OAuth2Token, error) {
	return config.Exchange(ctx, code, codeVerifier)
}

func (XOAuth2API) Refresh(ctx context.Context, config pkgx.OAuth2Config, refreshToken string) (pkgx.OAuth2Token, error) {
	return config.Refresh(ctx, refreshToken)
}

func (XOAuth2API) Me(ctx context.Context, accessToken string) (pkgx.User, error) {
	return pkgx.NewOAuth2Client(pkgx.OAuth2Credentials{AccessToken: accessToken}).Me(ctx)
}

func (s *Service) StartXOAuth2(userID uuid.UUID, redirectURI string) (string, error) {
	config, err := xOAuth2ConfigFromEnv(redirectURI)
	if err != nil {
		return "", err
	}

	codeVerifier, err := pkgx.GenerateOAuth2CodeVerifier()
	if err != nil {
		return "", err
	}
	state, err := newXOAuth2State()
	if err != nil {
		return "", err
	}

	authURL, err := s.xOAuth2Provider.AuthorizationURL(
		config,
		state,
		pkgx.OAuth2CodeChallengeS256(codeVerifier),
	)
	if err != nil {
		return "", err
	}

	pending := xOAuth2PendingState{
		UserID:       userID,
		CodeVerifier: codeVerifier,
		RedirectURI:  strings.TrimSpace(redirectURI),
		ExpiresAt:    time.Now().Add(xOAuth2StateTTL),
	}
	if err := s.xOAuth2States.Store(context.Background(), state, pending, xOAuth2StateTTL); err != nil {
		return "", err
	}
	return authURL, nil
}

func (s *Service) CompleteXOAuth2(ctx context.Context, state, code string) (*dto.XAccountResponse, error) {
	pending, ok, err := s.xOAuth2States.Consume(ctx, strings.TrimSpace(state))
	if err != nil {
		return nil, err
	}
	if !ok || time.Now().After(pending.ExpiresAt) {
		return nil, ErrInvalidXOAuth2State
	}

	config, err := xOAuth2ConfigFromEnv(pending.RedirectURI)
	if err != nil {
		return nil, err
	}
	token, err := s.xOAuth2Provider.Exchange(ctx, config, code, pending.CodeVerifier)
	if err != nil {
		return nil, err
	}
	user, err := s.xOAuth2Provider.Me(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}

	return s.saveXOAuth2Account(pending.UserID, token, user)
}

func (s *Service) saveXOAuth2Account(userID uuid.UUID, token pkgx.OAuth2Token, user pkgx.User) (*dto.XAccountResponse, error) {
	var account models.PlatformAccount
	err := s.db.Where("user_id = ? AND platform = ?", userID, xPlatform).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	credentials, err := parseXCredentials(account.Credentials)
	if err != nil {
		return nil, err
	}

	credentials.AuthType = xAuthTypeOAuth2
	credentials.APIKey = ""
	credentials.APISecret = ""
	credentials.AccessToken = ""
	credentials.AccessTokenSecret = ""
	credentials.OAuth2AccessToken = token.AccessToken
	credentials.OAuth2RefreshToken = firstNonEmpty(token.RefreshToken, credentials.OAuth2RefreshToken)
	if !token.ExpiresAt.IsZero() {
		expiresAt := token.ExpiresAt
		credentials.OAuth2ExpiresAt = &expiresAt
	}
	credentials.OAuth2Scope = token.Scope
	credentials.Username = user.Username

	rawCredentials, err := marshalJSON(credentials)
	if err != nil {
		return nil, err
	}
	metadata, err := marshalJSON(xMetadata{
		Name:     user.Name,
		UserID:   user.ID,
		Username: user.Username,
	})
	if err != nil {
		return nil, err
	}

	testedAt := time.Now().UTC()
	if account.ID == uuid.Nil {
		account = models.PlatformAccount{
			UserID:        userID,
			Platform:      xPlatform,
			Username:      "X",
			Credentials:   rawCredentials,
			Metadata:      metadata,
			Status:        models.PlatformAccountStatusConnected,
			LastTestedAt:  &testedAt,
			LastTestError: "",
		}
		err = s.db.Create(&account).Error
	} else {
		err = s.db.Model(&account).Updates(map[string]interface{}{
			"username":        "X",
			"credentials":     rawCredentials,
			"metadata":        datatypes.JSON(metadata),
			"status":          models.PlatformAccountStatusConnected,
			"last_tested_at":  &testedAt,
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

func xOAuth2ConfigFromEnv(redirectURI string) (pkgx.OAuth2Config, error) {
	clientID := strings.TrimSpace(os.Getenv(xOAuth2ClientIDEnv))
	redirectURI = strings.TrimSpace(redirectURI)
	if clientID == "" || redirectURI == "" {
		return pkgx.OAuth2Config{}, fmt.Errorf("%w: X_OAUTH2_CLIENT_ID and redirect_uri are required", ErrXOAuth2NotConfigured)
	}

	return pkgx.OAuth2Config{
		ClientID:     clientID,
		ClientSecret: strings.TrimSpace(os.Getenv(xOAuth2ClientSecretEnv)),
		RedirectURI:  redirectURI,
		AuthorizeURL: strings.TrimSpace(os.Getenv(xOAuth2AuthorizeURLEnv)),
		TokenURL:     strings.TrimSpace(os.Getenv(xOAuth2TokenURLEnv)),
	}, nil
}

func xOAuth2RefreshConfigFromEnv() (pkgx.OAuth2Config, error) {
	clientID := strings.TrimSpace(os.Getenv(xOAuth2ClientIDEnv))
	if clientID == "" {
		return pkgx.OAuth2Config{}, fmt.Errorf("%w: X_OAUTH2_CLIENT_ID is required", ErrXOAuth2NotConfigured)
	}

	return pkgx.OAuth2Config{
		ClientID:     clientID,
		ClientSecret: strings.TrimSpace(os.Getenv(xOAuth2ClientSecretEnv)),
		AuthorizeURL: strings.TrimSpace(os.Getenv(xOAuth2AuthorizeURLEnv)),
		TokenURL:     strings.TrimSpace(os.Getenv(xOAuth2TokenURLEnv)),
	}, nil
}

func shouldRefreshXOAuth2Credentials(credentials xCredentials) bool {
	if strings.TrimSpace(credentials.OAuth2AccessToken) == "" {
		return strings.TrimSpace(credentials.OAuth2RefreshToken) != ""
	}
	if credentials.OAuth2ExpiresAt == nil {
		return false
	}
	return time.Now().Add(xOAuth2RefreshSkew).After(*credentials.OAuth2ExpiresAt)
}

func (s *Service) refreshXOAuth2CredentialsIfNeeded(ctx context.Context, account *models.PlatformAccount, credentials xCredentials) (xCredentials, error) {
	if !shouldRefreshXOAuth2Credentials(credentials) {
		return credentials, nil
	}
	if strings.TrimSpace(credentials.OAuth2RefreshToken) == "" {
		return credentials, fmt.Errorf("%w: x oauth2 access token expired and refresh token is missing", ErrInvalidPlatformAccount)
	}

	config, err := xOAuth2RefreshConfigFromEnv()
	if err != nil {
		return credentials, err
	}
	token, err := s.xOAuth2Provider.Refresh(ctx, config, credentials.OAuth2RefreshToken)
	if err != nil {
		return credentials, fmt.Errorf("failed to refresh x oauth2 token: %w", err)
	}

	credentials.OAuth2AccessToken = token.AccessToken
	credentials.OAuth2RefreshToken = firstNonEmpty(token.RefreshToken, credentials.OAuth2RefreshToken)
	if !token.ExpiresAt.IsZero() {
		expiresAt := token.ExpiresAt
		credentials.OAuth2ExpiresAt = &expiresAt
	}
	credentials.OAuth2Scope = firstNonEmpty(token.Scope, credentials.OAuth2Scope)

	rawCredentials, err := marshalJSON(credentials)
	if err != nil {
		return credentials, err
	}
	if err := s.db.Model(account).Update("credentials", rawCredentials).Error; err != nil {
		return credentials, err
	}
	account.Credentials = rawCredentials
	return credentials, nil
}

func newXOAuth2State() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
