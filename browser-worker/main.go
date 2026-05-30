package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Reusing types from the design (simplified for implementation)
type DomainRule struct {
	Host    string   `json:"host"`
	Match   string   `json:"match"` // "exact" or "suffix"
	Schemes []string `json:"schemes"`
	Purpose string   `json:"purpose"`
}

type StartWorkerSessionRequest struct {
	SessionID      uuid.UUID    `json:"session_id"`
	UserID         uuid.UUID    `json:"user_id"`
	Platform       string       `json:"platform"`
	LoginURL       string       `json:"login_url"`
	AllowedDomains []DomainRule `json:"allowed_domains"`
	TTLSeconds     int          `json:"ttl_seconds"`
}

type StartWorkerSessionResponse struct {
	WorkerSessionRef  string    `json:"worker_session_ref"`
	Status            string    `json:"status"`
	ContainerID       string    `json:"container_id"`
	CDPEndpointRef    string    `json:"cdp_endpoint_ref"`
	StreamEndpointRef string    `json:"stream_endpoint_ref"`
	StartedAt         time.Time `json:"started_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type WorkerSession struct {
	ID                string
	SessionID         uuid.UUID
	UserID            uuid.UUID
	Platform          string
	Status            string
	CDPEndpointRef    string
	StreamEndpointRef string
	ExpiresAt         time.Time
	// In a real implementation, we'd store the docker container ID here
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*WorkerSession
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*WorkerSession),
	}
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	sm := NewSessionManager()

	e.POST("/internal/browser-sessions", func(c echo.Context) error {
		var req StartWorkerSessionRequest
		if err := c.Bind(&req); err != nil {
			return err
		}

		ref := uuid.NewString()
		now := time.Now()
		expiresAt := now.Add(time.Duration(req.TTLSeconds) * time.Second)

		session := &WorkerSession{
			ID:                ref,
			SessionID:         req.SessionID,
			UserID:            req.UserID,
			Platform:          req.Platform,
			Status:            "ready",
			CDPEndpointRef:    "ws://localhost:9222", // Placeholder
			StreamEndpointRef: "ws://localhost:6080", // Placeholder
			ExpiresAt:         expiresAt,
		}

		sm.mu.Lock()
		sm.sessions[ref] = session
		sm.mu.Unlock()

		return c.JSON(http.StatusCreated, StartWorkerSessionResponse{
			WorkerSessionRef:  ref,
			Status:            session.Status,
			CDPEndpointRef:    session.CDPEndpointRef,
			StreamEndpointRef: session.StreamEndpointRef,
			StartedAt:         now,
			ExpiresAt:         expiresAt,
		})
	})

	e.GET("/internal/browser-sessions/:ref", func(c echo.Context) error {
		ref := c.Param("ref")
		sm.mu.RLock()
		session, ok := sm.sessions[ref]
		sm.mu.RUnlock()

		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"worker_session_ref": ref,
			"status":             session.Status,
			"expires_at":         session.ExpiresAt,
		})
	})

	e.Logger.Fatal(e.Start(":8081"))
}
