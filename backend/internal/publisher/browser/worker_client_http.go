package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

type HttpBrowserWorkerClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHttpBrowserWorkerClient(baseURL string) *HttpBrowserWorkerClient {
	return &HttpBrowserWorkerClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: resilience.NewHTTPClient("browser-worker", 30*time.Second),
	}
}

func (c *HttpBrowserWorkerClient) CreateSession(ctx context.Context, req StartWorkerSessionRequest) (*StartWorkerSessionResponse, error) {
	body, _ := json.Marshal(req)
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/browser-sessions", bytes.NewReader(body))
	hReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
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

func (c *HttpBrowserWorkerClient) GetSession(ctx context.Context, ref string) (*GetWorkerSessionResponse, error) {
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/browser-sessions/"+ref, nil)

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var result GetWorkerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HttpBrowserWorkerClient) CaptureSession(ctx context.Context, ref string) (*CaptureWorkerSessionResponse, error) {
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/browser-sessions/"+ref+"/capture", nil)

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var result CaptureWorkerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HttpBrowserWorkerClient) StartDouyinPublish(ctx context.Context, ref string, req StartDouyinPublishRequest) error {
	body, _ := json.Marshal(req)
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/browser-sessions/"+ref+"/publish/douyin", bytes.NewReader(body))
	hReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		var errResp struct {
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Message != "" {
			return fmt.Errorf("worker error: %s", errResp.Message)
		}
		return fmt.Errorf("worker returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *HttpBrowserWorkerClient) StopSession(ctx context.Context, ref string) error {
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/internal/browser-sessions/"+ref, nil)

	resp, err := c.httpClient.Do(hReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("worker returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *HttpBrowserWorkerClient) absoluteWorkerURL(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "ws://") || strings.HasPrefix(ref, "wss://") {
		return ref
	}
	if strings.HasPrefix(ref, "/") {
		return c.baseURL + ref
	}
	return c.baseURL + "/" + ref
}
