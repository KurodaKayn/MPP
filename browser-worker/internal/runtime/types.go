package runtime

import (
	"context"
	"fmt"
	"net"
	"time"
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
	TTL       time.Duration
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

type Manager interface {
	RuntimeDriver() string
	StartSession(ctx context.Context, request StartSessionRequest) (SessionReference, error)
	StopSession(ctx context.Context, reference SessionReference) error
}

type ExpiredSessionReapReport struct {
	Driver          string
	DeletedSessions int
	OldestExpiredAt time.Time
}

func (r ExpiredSessionReapReport) CleanupLag(now time.Time) time.Duration {
	if r.OldestExpiredAt.IsZero() {
		return 0
	}
	lag := now.Sub(r.OldestExpiredAt)
	if lag < 0 {
		return 0
	}
	return lag
}

type ExpiredSessionReaper interface {
	ReapExpiredSessions(ctx context.Context) (ExpiredSessionReapReport, error)
}
