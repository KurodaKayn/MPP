package x

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuth2CodeChallengeUsesS256(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	assert.Equal(t, "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", OAuth2CodeChallengeS256(verifier))
}

func TestOAuth2AuthorizationURLIncludesPKCEParams(t *testing.T) {
	authURL, err := OAuth2Config{
		ClientID:     "client-id",
		RedirectURI:  "https://app.example.com/api/user/dashboard/settings/x/oauth2/callback",
		AuthorizeURL: "https://x.example.com/i/oauth2/authorize",
	}.AuthorizationURL("state-value", "challenge-value")
	require.NoError(t, err)

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)

	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "x.example.com", parsed.Host)
	assert.Equal(t, "/i/oauth2/authorize", parsed.Path)

	query := parsed.Query()
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Equal(t, "client-id", query.Get("client_id"))
	assert.Equal(t, "state-value", query.Get("state"))
	assert.Equal(t, "challenge-value", query.Get("code_challenge"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.Equal(t, strings.Join(DefaultOAuth2Scopes, " "), query.Get("scope"))
}

func TestOAuth2ExchangePostsAuthorizationCodeGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "auth-code", r.Form.Get("code"))
		assert.Equal(t, "code-verifier", r.Form.Get("code_verifier"))
		assert.Equal(t, "https://app.example.com/callback", r.Form.Get("redirect_uri"))
		assert.Equal(t, "client-id", r.Form.Get("client_id"))

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"token_type":    "bearer",
			"scope":         "tweet.read tweet.write users.read offline.access",
			"expires_in":    7200,
		})
	}))
	defer server.Close()

	token, err := OAuth2Config{
		ClientID:    "client-id",
		RedirectURI: "https://app.example.com/callback",
		TokenURL:    server.URL,
	}.Exchange(context.Background(), "auth-code", "code-verifier")
	require.NoError(t, err)

	assert.Equal(t, "access-token", token.AccessToken)
	assert.Equal(t, "refresh-token", token.RefreshToken)
	assert.Equal(t, "bearer", token.TokenType)
	assert.Equal(t, 7200, token.ExpiresIn)
	assert.False(t, token.ExpiresAt.IsZero())
}

func TestOAuth2RefreshUsesClientSecretBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
		assert.Equal(t, expected, r.Header.Get("Authorization"))
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "refresh-token", r.Form.Get("refresh_token"))
		assert.Empty(t, r.Form.Get("client_id"))

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    7200,
		})
	}))
	defer server.Close()

	token, err := OAuth2Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "https://app.example.com/callback",
		TokenURL:     server.URL,
	}.Refresh(context.Background(), "refresh-token")
	require.NoError(t, err)

	assert.Equal(t, "new-access-token", token.AccessToken)
	assert.Equal(t, "new-refresh-token", token.RefreshToken)
}

func TestOAuth2ClientUsesBearerAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "/2/tweets", r.URL.Path)

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{
				"id":   "tweet-1",
				"text": "hello",
			},
		})
	}))
	defer server.Close()

	client := NewOAuth2Client(OAuth2Credentials{AccessToken: "access-token"})
	client.baseURL = server.URL

	tweet, err := client.CreateTweet(context.Background(), "hello")
	require.NoError(t, err)

	assert.Equal(t, "tweet-1", tweet.ID)
	assert.Equal(t, "hello", tweet.Text)
}
