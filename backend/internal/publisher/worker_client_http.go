package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type HttpBrowserWorkerClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHttpBrowserWorkerClient(baseURL string) *HttpBrowserWorkerClient {
	return &HttpBrowserWorkerClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
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
		return nil, fmt.Errorf("worker returned status %d", resp.StatusCode)
	}

	var result StartWorkerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
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
