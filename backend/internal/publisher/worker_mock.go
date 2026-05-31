package publisher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MockBrowserWorkerClient struct {
	mu             sync.RWMutex
	sessions       map[string]*StartWorkerSessionResponse
	streamBaseURL  string
	streamServer   *http.Server
	streamListener net.Listener
}

func NewMockBrowserWorkerClient() *MockBrowserWorkerClient {
	m := &MockBrowserWorkerClient{
		sessions: make(map[string]*StartWorkerSessionResponse),
	}
	m.startStreamServer()
	return m
}

func (m *MockBrowserWorkerClient) CreateSession(ctx context.Context, req StartWorkerSessionRequest) (*StartWorkerSessionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ref := "worker-" + uuid.NewString()
	streamEndpointRef := "ws://private-stream/" + ref
	if m.streamBaseURL != "" {
		streamEndpointRef = fmt.Sprintf("%s/stream/%s", m.streamBaseURL, ref)
	}
	resp := &StartWorkerSessionResponse{
		WorkerSessionRef:  ref,
		Status:            "ready",
		ContainerID:       "container-" + uuid.NewString(),
		CDPEndpointRef:    "ws://private-cdp/" + ref,
		StreamEndpointRef: streamEndpointRef,
		StartedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(time.Duration(req.TTLSeconds) * time.Second),
	}
	m.sessions[ref] = resp
	return resp, nil
}

func (m *MockBrowserWorkerClient) GetSession(ctx context.Context, ref string) (*GetWorkerSessionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[ref]
	if !ok {
		return nil, errors.New("session not found")
	}

	return &GetWorkerSessionResponse{
		WorkerSessionRef: ref,
		Status:           sess.Status,
		CurrentURL:       "https://login.example.com",
		LoginDetected:    false,
	}, nil
}

func (m *MockBrowserWorkerClient) CaptureSession(ctx context.Context, ref string) (*CaptureWorkerSessionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.sessions[ref]; !ok {
		return nil, errors.New("session not found")
	}

	return &CaptureWorkerSessionResponse{
		Status: "login_detected",
		Cookies: []Cookie{
			{Name: "sessionid", Value: "mock-value", Domain: ".example.com"},
		},
		Account: RemoteAccountProfile{
			Username: "Mock User",
		},
	}, nil
}

func (m *MockBrowserWorkerClient) StopSession(ctx context.Context, ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, ref)
	return nil
}

func (m *MockBrowserWorkerClient) Close() error {
	if m.streamServer == nil {
		return nil
	}
	return m.streamServer.Close()
}

func (m *MockBrowserWorkerClient) startStreamServer() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", mockStreamHandler)
	m.streamServer = &http.Server{Handler: mux}
	m.streamListener = listener
	m.streamBaseURL = "http://" + listener.Addr().String()
	go func() {
		_ = m.streamServer.Serve(listener)
	}()
}

func mockStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Mock remote browser</title></head>
<body style="margin:0;font-family:system-ui,sans-serif;background:#111827;color:#f9fafb;display:grid;place-items:center;min-height:100vh">
<main style="max-width:520px;padding:24px;text-align:center">
<h1>Mock remote browser session</h1>
<p>This development mock does not open a real noVNC browser. Configure BROWSER_WORKER_URL to use the isolated Chromium worker.</p>
</main>
</body>
</html>`))
}
