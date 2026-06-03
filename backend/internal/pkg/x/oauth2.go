package x

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

const (
	defaultOAuth2AuthorizeURL = "https://x.com/i/oauth2/authorize"
	defaultOAuth2TokenURL     = "https://api.x.com/2/oauth2/token"
)

var (
	ErrMissingOAuth2Config      = errors.New("x oauth2 config is required")
	ErrMissingOAuth2Credentials = errors.New("x oauth2 credentials are required")
)

var DefaultOAuth2Scopes = []string{
	"tweet.read",
	"tweet.write",
	"users.read",
	"offline.access",
}

type OAuth2Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	AuthorizeURL string
	TokenURL     string
	HTTPClient   *http.Client
}

type OAuth2Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresIn    int
	ExpiresAt    time.Time
}

type oauth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
}

type OAuth2Credentials struct {
	AccessToken string
}

type OAuth2Client struct {
	baseURL    string
	httpClient *http.Client
	creds      OAuth2Credentials
}

func GenerateOAuth2CodeVerifier() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func OAuth2CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c OAuth2Config) AuthorizationURL(state, codeChallenge string) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(state) == "" || strings.TrimSpace(codeChallenge) == "" {
		return "", fmt.Errorf("%w: state and code_challenge are required", ErrMissingOAuth2Config)
	}

	endpoint, err := url.Parse(firstNonEmptyString(c.AuthorizeURL, defaultOAuth2AuthorizeURL))
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", strings.TrimSpace(c.ClientID))
	query.Set("redirect_uri", strings.TrimSpace(c.RedirectURI))
	query.Set("scope", strings.Join(c.scopes(), " "))
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (c OAuth2Config) Exchange(ctx context.Context, code, codeVerifier string) (OAuth2Token, error) {
	code = strings.TrimSpace(code)
	codeVerifier = strings.TrimSpace(codeVerifier)
	if err := c.validate(); err != nil {
		return OAuth2Token{}, err
	}
	if code == "" || codeVerifier == "" {
		return OAuth2Token{}, fmt.Errorf("%w: code and code_verifier are required", ErrMissingOAuth2Config)
	}

	values := url.Values{
		"grant_type":    []string{"authorization_code"},
		"code":          []string{code},
		"redirect_uri":  []string{strings.TrimSpace(c.RedirectURI)},
		"code_verifier": []string{codeVerifier},
	}
	return c.tokenRequest(ctx, values)
}

func (c OAuth2Config) Refresh(ctx context.Context, refreshToken string) (OAuth2Token, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if err := c.validateClient(); err != nil {
		return OAuth2Token{}, err
	}
	if refreshToken == "" {
		return OAuth2Token{}, fmt.Errorf("%w: refresh_token is required", ErrMissingOAuth2Credentials)
	}

	values := url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
	}
	return c.tokenRequest(ctx, values)
}

func NewOAuth2Client(creds OAuth2Credentials) *OAuth2Client {
	return &OAuth2Client{
		baseURL:    defaultBaseURL,
		creds:      creds,
		httpClient: resilience.NewHTTPClient("x", 20*time.Second),
	}
}

func (c *OAuth2Client) Me(ctx context.Context) (User, error) {
	var out userResponse
	err := c.doJSON(ctx, http.MethodGet, "/2/users/me", url.Values{
		"user.fields": []string{"username,name"},
	}, nil, &out)
	if err != nil {
		return User{}, err
	}
	return out.Data, nil
}

func (c *OAuth2Client) CreateTweet(ctx context.Context, text string) (Tweet, error) {
	body := map[string]string{"text": text}
	var out tweetResponse
	err := c.doJSON(ctx, http.MethodPost, "/2/tweets", nil, body, &out)
	if err != nil {
		return Tweet{}, err
	}
	return out.Data, nil
}

func (c OAuth2Config) validate() error {
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.RedirectURI) == "" {
		return fmt.Errorf("%w: client_id and redirect_uri are required", ErrMissingOAuth2Config)
	}
	return nil
}

func (c OAuth2Config) validateClient() error {
	if strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("%w: client_id is required", ErrMissingOAuth2Config)
	}
	return nil
}

func (c OAuth2Config) scopes() []string {
	if len(c.Scopes) == 0 {
		return DefaultOAuth2Scopes
	}
	return c.Scopes
}

func (c OAuth2Config) tokenRequest(ctx context.Context, values url.Values) (OAuth2Token, error) {
	if err := c.validateClient(); err != nil {
		return OAuth2Token{}, err
	}

	if strings.TrimSpace(c.ClientSecret) == "" {
		values.Set("client_id", strings.TrimSpace(c.ClientID))
	}

	tokenURL := firstNonEmptyString(c.TokenURL, defaultOAuth2TokenURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return OAuth2Token{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if strings.TrimSpace(c.ClientSecret) != "" {
		req.SetBasicAuth(strings.TrimSpace(c.ClientID), strings.TrimSpace(c.ClientSecret))
	}

	client := c.HTTPClient
	if client == nil {
		client = resilience.NewHTTPClient("x-oauth2", 20*time.Second)
	}

	resp, err := client.Do(req)
	if err != nil {
		return OAuth2Token{}, fmt.Errorf("x oauth2 token request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return OAuth2Token{}, fmt.Errorf("failed to read x oauth2 token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OAuth2Token{}, parseAPIError(resp.StatusCode, responseBody)
	}

	var out oauth2TokenResponse
	if err := json.Unmarshal(responseBody, &out); err != nil {
		return OAuth2Token{}, fmt.Errorf("failed to parse x oauth2 token response: %w", err)
	}
	token := OAuth2Token{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		TokenType:    out.TokenType,
		Scope:        out.Scope,
		ExpiresIn:    out.ExpiresIn,
	}
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return OAuth2Token{}, fmt.Errorf("%w: access_token is required", ErrMissingOAuth2Credentials)
	}
	return token, nil
}

func (c *OAuth2Client) doJSON(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}) error {
	if err := c.creds.Validate(); err != nil {
		return err
	}

	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.creds.AccessToken))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("x api request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read x api response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, responseBody)
	}
	if len(responseBody) == 0 || out == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("failed to parse x api response: %w", err)
	}
	return nil
}

func (c OAuth2Credentials) Validate() error {
	if strings.TrimSpace(c.AccessToken) == "" {
		return ErrMissingOAuth2Credentials
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
