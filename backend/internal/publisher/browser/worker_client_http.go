package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

type HTTPBrowserWorkerClient struct {
	baseURL       string
	httpClient    *http.Client
	internalToken string
}

func NewHTTPBrowserWorkerClient(baseURL string) *HTTPBrowserWorkerClient {
	return NewHTTPBrowserWorkerClientWithToken(baseURL, "")
}

func NewHTTPBrowserWorkerClientWithToken(baseURL, internalToken string) *HTTPBrowserWorkerClient {
	return &HTTPBrowserWorkerClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		httpClient:    resilience.NewHTTPClient("browser-worker", 30*time.Second),
		internalToken: strings.TrimSpace(internalToken),
	}
}

func (c *HTTPBrowserWorkerClient) CreateSession(ctx context.Context, req StartWorkerSessionRequest) (*StartWorkerSessionResponse, error) {
	body, _ := json.Marshal(req)
	hReq, _ := c.newRequest(ctx, http.MethodPost, "/internal/browser-sessions", bytes.NewReader(body))
	hReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests {
			if errResp.Message != "" {
				return nil, fmt.Errorf("%w: %s", ErrBrowserWorkerPoolExhausted, errResp.Message)
			}
			return nil, ErrBrowserWorkerPoolExhausted
		}
		if errResp.Message != "" {
			return nil, fmt.Errorf("worker error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var result StartWorkerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	result.StreamEndpointRef = c.absoluteWorkerURL(result.StreamEndpointRef)
	return &result, nil
}

func (c *HTTPBrowserWorkerClient) GetSession(ctx context.Context, ref string) (*GetWorkerSessionResponse, error) {
	hReq, _ := c.newRequest(ctx, http.MethodGet, "/internal/browser-sessions/"+ref, nil)

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var result GetWorkerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPBrowserWorkerClient) CaptureSession(ctx context.Context, ref string) (*CaptureWorkerSessionResponse, error) {
	hReq, _ := c.newRequest(ctx, http.MethodPost, "/internal/browser-sessions/"+ref+"/capture", nil)

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var result CaptureWorkerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPBrowserWorkerClient) StartDouyinPublish(ctx context.Context, ref string, req StartDouyinPublishRequest) error {
	body, _ := json.Marshal(req)
	hReq, _ := c.newRequest(ctx, http.MethodPost, "/internal/browser-sessions/"+ref+"/publish/douyin", bytes.NewReader(body))
	hReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		var errResp struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Message != "" {
			return fmt.Errorf("worker error: %s", errResp.Message)
		}
		return fmt.Errorf("worker returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *HTTPBrowserWorkerClient) StopSession(ctx context.Context, ref string) error {
	hReq, _ := c.newRequest(ctx, http.MethodDelete, "/internal/browser-sessions/"+ref, nil)

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("worker returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *HTTPBrowserWorkerClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.internalToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.internalToken)
	}
	return req, nil
}

func (c *HTTPBrowserWorkerClient) absoluteWorkerURL(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "ws://") || strings.HasPrefix(ref, "wss://") {
		return ref
	}
	if strings.HasPrefix(ref, "/") {
		return c.baseURL + ref
	}
	return c.baseURL + "/" + ref
}
