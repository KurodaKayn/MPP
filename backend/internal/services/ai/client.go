package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

const (
	aiServiceURLEnv     = "AI_SERVICE_URL"
	defaultAIServiceURL = "http://localhost:8000"
	aiServiceTimeout    = 90 * time.Second
)

var (
	ErrAIServiceUnavailable = errors.New("ai service unavailable")
	ErrInvalidAIEditRequest = errors.New("invalid ai edit request")
)

type AIContentEditor interface {
	EditContent(ctx context.Context, req dto.AIEditContentRequest) (*dto.AIEditContentResponse, error)
	EditPrepublish(ctx context.Context, req dto.AIEditPrepublishRequest) (*dto.AIEditPrepublishResponse, error)
	StreamEditContent(ctx context.Context, req dto.AIEditContentRequest) (*AIServiceStream, error)
	StreamEditPrepublish(ctx context.Context, req dto.AIEditPrepublishRequest) (*AIServiceStream, error)
}

type AIServiceStream struct {
	Body        io.ReadCloser
	ContentType string
}

type AIServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAIServiceClientFromEnv() *AIServiceClient {
	baseURL := strings.TrimSpace(os.Getenv(aiServiceURLEnv))
	if baseURL == "" {
		baseURL = defaultAIServiceURL
	}
	return NewAIServiceClient(baseURL, nil)
}

func NewAIServiceClient(baseURL string, httpClient *http.Client) *AIServiceClient {
	if httpClient == nil {
		httpClient = resilience.NewHTTPClient("ai-service", aiServiceTimeout)
	}
	return &AIServiceClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: httpClient,
	}
}

func (c *AIServiceClient) EditContent(ctx context.Context, req dto.AIEditContentRequest) (*dto.AIEditContentResponse, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, ErrInvalidAIEditRequest
	}

	var resp dto.AIEditContentResponse
	if err := c.postJSON(ctx, "/content/edit", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AIServiceClient) StreamEditContent(ctx context.Context, req dto.AIEditContentRequest) (*AIServiceStream, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, ErrInvalidAIEditRequest
	}
	return c.postStream(ctx, "/content/edit/stream", req)
}

func (c *AIServiceClient) EditPrepublish(ctx context.Context, req dto.AIEditPrepublishRequest) (*dto.AIEditPrepublishResponse, error) {
	if strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.Message) == "" || len(req.AdaptedContent) == 0 {
		return nil, ErrInvalidAIEditRequest
	}

	var resp dto.AIEditPrepublishResponse
	if err := c.postJSON(ctx, "/prepublish/edit", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AIServiceClient) StreamEditPrepublish(ctx context.Context, req dto.AIEditPrepublishRequest) (*AIServiceStream, error) {
	if strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.Message) == "" || len(req.AdaptedContent) == 0 {
		return nil, ErrInvalidAIEditRequest
	}
	return c.postStream(ctx, "/prepublish/edit/stream", req)
}

func (c *AIServiceClient) postJSON(ctx context.Context, path string, payload interface{}, out interface{}) error {
	if c == nil || c.baseURL == "" {
		return ErrAIServiceUnavailable
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAIServiceUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return aiServiceStatusError(resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: invalid response: %v", ErrAIServiceUnavailable, err)
	}
	return nil
}

func (c *AIServiceClient) postStream(ctx context.Context, path string, payload interface{}) (*AIServiceStream, error) {
	if c == nil || c.baseURL == "" {
		return nil, ErrAIServiceUnavailable
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/markdown, text/plain, application/octet-stream")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAIServiceUnavailable, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		return nil, aiServiceStatusError(resp)
	}

	return &AIServiceStream{
		Body:        resp.Body,
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

func aiServiceStatusError(resp *http.Response) error {
	message := strings.TrimSpace(readAIServiceErrorMessage(resp.Body))
	if message == "" {
		message = fmt.Sprintf("returned status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("%w: %s", ErrInvalidAIEditRequest, message)
	}
	return fmt.Errorf("%w: %s", ErrAIServiceUnavailable, message)
}

func readAIServiceErrorMessage(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil || len(raw) == 0 {
		return ""
	}

	var parsed struct {
		Detail  interface{} `json:"detail"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return string(raw)
	}
	if parsed.Message != "" {
		return parsed.Message
	}
	if detail, ok := parsed.Detail.(string); ok {
		return detail
	}
	if parsed.Detail != nil {
		rendered, err := json.Marshal(parsed.Detail)
		if err == nil {
			return string(rendered)
		}
	}
	return ""
}
