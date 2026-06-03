package x

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

const defaultBaseURL = "https://api.x.com"

var ErrMissingCredentials = errors.New("x api credentials are required")

type Credentials struct {
	APIKey            string
	APISecret         string
	AccessToken       string
	AccessTokenSecret string
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	creds      Credentials
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type Tweet struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type userResponse struct {
	Data User `json:"data"`
}

type tweetResponse struct {
	Data Tweet `json:"data"`
}

type apiProblem struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
}

type apiErrorResponse struct {
	Detail string       `json:"detail"`
	Errors []apiProblem `json:"errors"`
	Title  string       `json:"title"`
	Type   string       `json:"type"`
}

func NewClient(creds Credentials) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		creds:      creds,
		httpClient: resilience.NewHTTPClient("x", 20*time.Second),
	}
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var out userResponse
	err := c.doJSON(ctx, http.MethodGet, "/2/users/me", url.Values{
		"user.fields": []string{"username,name"},
	}, nil, &out)
	if err != nil {
		return User{}, err
	}
	return out.Data, nil
}

func (c *Client) CreateTweet(ctx context.Context, text string) (Tweet, error) {
	body := map[string]string{"text": text}
	var out tweetResponse
	err := c.doJSON(ctx, http.MethodPost, "/2/tweets", nil, body, &out)
	if err != nil {
		return Tweet{}, err
	}
	return out.Data, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}) error {
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
	req.Header.Set("Authorization", SignOAuth1(method, endpoint.String(), c.creds))
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

func (c Credentials) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" ||
		strings.TrimSpace(c.APISecret) == "" ||
		strings.TrimSpace(c.AccessToken) == "" ||
		strings.TrimSpace(c.AccessTokenSecret) == "" {
		return ErrMissingCredentials
	}
	return nil
}

func SignOAuth1(method, rawURL string, creds Credentials) string {
	oauthParams := map[string]string{
		"oauth_consumer_key":     creds.APIKey,
		"oauth_nonce":            nonce(),
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        fmt.Sprintf("%d", time.Now().Unix()),
		"oauth_token":            creds.AccessToken,
		"oauth_version":          "1.0",
	}

	parsed, _ := url.Parse(rawURL)
	signingParams := make([]oauthPair, 0, len(oauthParams)+len(parsed.Query()))
	for key, value := range oauthParams {
		signingParams = append(signingParams, oauthPair{key: key, value: value})
	}
	for key, values := range parsed.Query() {
		for _, value := range values {
			signingParams = append(signingParams, oauthPair{key: key, value: value})
		}
	}
	sort.Slice(signingParams, func(i, j int) bool {
		leftKey := percentEncode(signingParams[i].key)
		rightKey := percentEncode(signingParams[j].key)
		if leftKey == rightKey {
			return percentEncode(signingParams[i].value) < percentEncode(signingParams[j].value)
		}
		return leftKey < rightKey
	})

	encodedPairs := make([]string, 0, len(signingParams))
	for _, param := range signingParams {
		encodedPairs = append(encodedPairs, percentEncode(param.key)+"="+percentEncode(param.value))
	}

	baseURL := *parsed
	baseURL.RawQuery = ""
	baseURL.Fragment = ""
	signatureBase := strings.Join([]string{
		strings.ToUpper(method),
		percentEncode(baseURL.String()),
		percentEncode(strings.Join(encodedPairs, "&")),
	}, "&")
	signingKey := percentEncode(creds.APISecret) + "&" + percentEncode(creds.AccessTokenSecret)

	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(signatureBase))
	oauthParams["oauth_signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	headerPairs := make([]oauthPair, 0, len(oauthParams))
	for key, value := range oauthParams {
		headerPairs = append(headerPairs, oauthPair{key: key, value: value})
	}
	sort.Slice(headerPairs, func(i, j int) bool {
		return headerPairs[i].key < headerPairs[j].key
	})

	header := make([]string, 0, len(headerPairs))
	for _, pair := range headerPairs {
		header = append(header, percentEncode(pair.key)+`="`+percentEncode(pair.value)+`"`)
	}
	return "OAuth " + strings.Join(header, ", ")
}

type oauthPair struct {
	key   string
	value string
}

func nonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func percentEncode(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func parseAPIError(statusCode int, body []byte) error {
	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil {
		parts := make([]string, 0, 3)
		if apiErr.Title != "" {
			parts = append(parts, apiErr.Title)
		}
		if apiErr.Detail != "" {
			parts = append(parts, apiErr.Detail)
		}
		for _, item := range apiErr.Errors {
			if item.Title != "" {
				parts = append(parts, item.Title)
			}
			if item.Detail != "" {
				parts = append(parts, item.Detail)
			}
		}
		if len(parts) > 0 {
			return fmt.Errorf("x api returned %d: %s", statusCode, strings.Join(parts, "; "))
		}
	}

	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return fmt.Errorf("x api returned %d: %s", statusCode, message)
}
