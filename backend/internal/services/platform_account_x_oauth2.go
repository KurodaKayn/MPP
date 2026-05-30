package services

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

func (s *DashboardService) StartXOAuth2(userID uuid.UUID, redirectURI string) (string, error) {
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

	s.storeXOAuth2State(state, xOAuth2PendingState{
		UserID:       userID,
		CodeVerifier: codeVerifier,
		RedirectURI:  strings.TrimSpace(redirectURI),
		ExpiresAt:    time.Now().Add(xOAuth2StateTTL),
	})
	return authURL, nil
}

func (s *DashboardService) CompleteXOAuth2(ctx context.Context, state, code string) (*dto.XAccountResponse, error) {
	pending, ok := s.consumeXOAuth2State(strings.TrimSpace(state))
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

func (s *DashboardService) saveXOAuth2Account(userID uuid.UUID, token pkgx.OAuth2Token, user pkgx.User) (*dto.XAccountResponse, error) {
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
			Name:          "X",
			Credentials:   rawCredentials,
			Metadata:      metadata,
			Status:        models.PlatformAccountStatusConnected,
			LastTestedAt:  &testedAt,
			LastTestError: "",
		}
		err = s.db.Create(&account).Error
	} else {
		err = s.db.Model(&account).Updates(map[string]interface{}{
			"name":            "X",
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

func (s *DashboardService) storeXOAuth2State(state string, pending xOAuth2PendingState) {
	s.xOAuth2StatesMu.Lock()
	defer s.xOAuth2StatesMu.Unlock()

	now := time.Now()
	for existingState, existingPending := range s.xOAuth2States {
		if now.After(existingPending.ExpiresAt) {
			delete(s.xOAuth2States, existingState)
		}
	}
	s.xOAuth2States[state] = pending
}

func (s *DashboardService) consumeXOAuth2State(state string) (xOAuth2PendingState, bool) {
	s.xOAuth2StatesMu.Lock()
	defer s.xOAuth2StatesMu.Unlock()

	pending, ok := s.xOAuth2States[state]
	if ok {
		delete(s.xOAuth2States, state)
	}
	return pending, ok
}

func newXOAuth2State() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
