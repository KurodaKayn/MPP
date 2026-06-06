package runtime

import (
	"context"
	"fmt"
	"net"
)

const (
	DriverDocker     = "docker"
	DriverKubernetes = "kubernetes"
)

type Endpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type StartSessionRequest struct {
	SessionID string
	UserID    string
	Platform  string
}

type SessionReference struct {
	Driver         string            `json:"driver"`
	RuntimeID      string            `json:"runtime_id"`
	CDPEndpoint    Endpoint          `json:"cdp_endpoint"`
	StreamEndpoint Endpoint          `json:"stream_endpoint"`
	CleanupLabels  map[string]string `json:"cleanup_labels,omitempty"`
}

func (r SessionReference) IsZero() bool {
	return r.Driver == "" && r.RuntimeID == ""
}

func (r SessionReference) InternalStreamURL() string {
	return fmt.Sprintf("http://%s", net.JoinHostPort(r.StreamEndpoint.Host, fmt.Sprintf("%d", r.StreamEndpoint.Port)))
}

func (r SessionReference) LegacyContainerID() string {
	if r.Driver != DriverDocker {
		return ""
	}
	return r.RuntimeID
}

type Manager interface {
	StartSession(ctx context.Context, request StartSessionRequest) (SessionReference, error)
	StopSession(ctx context.Context, reference SessionReference) error
}
