package publisher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MockBrowserWorkerClient struct {
	mu       sync.RWMutex
	sessions map[string]*StartWorkerSessionResponse
}

func NewMockBrowserWorkerClient() *MockBrowserWorkerClient {
	return &MockBrowserWorkerClient{
		sessions: make(map[string]*StartWorkerSessionResponse),
	}
}

func (m *MockBrowserWorkerClient) CreateSession(ctx context.Context, req StartWorkerSessionRequest) (*StartWorkerSessionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ref := "worker-" + uuid.NewString()
	resp := &StartWorkerSessionResponse{
		WorkerSessionRef:  ref,
		Status:            "ready",
		ContainerID:       "container-" + uuid.NewString(),
		CDPEndpointRef:    "ws://private-cdp/" + ref,
		StreamEndpointRef: "ws://private-stream/" + ref,
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
