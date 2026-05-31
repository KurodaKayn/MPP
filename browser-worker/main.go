package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HttpOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
}

type RemoteAccountProfile struct {
	PlatformUserID string `json:"platform_user_id"`
	Username       string `json:"username"`
	AvatarURL      string `json:"avatar_url"`
}

type CaptureWorkerSessionResponse struct {
	Status  string               `json:"status"`
	Cookies []Cookie             `json:"cookies"`
	Account RemoteAccountProfile `json:"account"`
}

// Reusing types from the design (simplified for implementation)
type DomainRule struct {
	Host    string   `json:"host"`
	Match   string   `json:"match"` // "exact" or "suffix"
	Schemes []string `json:"schemes"`
	Purpose string   `json:"purpose"`
}

type CookieRequirement struct {
	Name           string   `json:"name"`
	DomainSuffixes []string `json:"domain_suffixes"`
	Required       bool     `json:"required"`
	Preserve       bool     `json:"preserve"`
}

type StartWorkerSessionRequest struct {
	SessionID       uuid.UUID           `json:"session_id"`
	UserID          uuid.UUID           `json:"user_id"`
	Platform        string              `json:"platform"`
	LoginURL        string              `json:"login_url"`
	AllowedDomains  []DomainRule        `json:"allowed_domains"`
	RequiredCookies []CookieRequirement `json:"required_cookies"`
	TTLSeconds      int                 `json:"ttl_seconds"`
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
	ContainerID       string
	CDPEndpointRef    string
	StreamEndpointRef string
	RequiredCookies   []CookieRequirement
	ExpiresAt         time.Time
	CancelFunc        context.CancelFunc
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

	dm, err := NewDockerManager()
	if err != nil {
		log.Fatalf("Failed to initialize Docker manager: %v", err)
	}

	sm := NewSessionManager()

	e.POST("/internal/browser-sessions", func(c echo.Context) error {
		var req StartWorkerSessionRequest
		if err := c.Bind(&req); err != nil {
			return err
		}

		// 1. Start Docker Container with Login URL
		containerID, _, cdpPort, streamPort, err := dm.StartBrowserContainer(c.Request().Context(), req.SessionID.String(), req.LoginURL)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to start browser: %v", err))
		}

		_, sessCancel := context.WithCancel(context.Background())

		workerSession := &WorkerSession{
			ID:                uuid.NewString(),
			SessionID:         req.SessionID,
			UserID:            req.UserID,
			Platform:          req.Platform,
			Status:            "ready",
			ContainerID:       containerID,
			CDPEndpointRef:    fmt.Sprintf("ws://localhost:%d", cdpPort),
			StreamEndpointRef: fmt.Sprintf("http://localhost:%d", streamPort),
			RequiredCookies:   req.RequiredCookies,
			ExpiresAt:         time.Now().Add(time.Duration(req.TTLSeconds) * time.Second),
			CancelFunc:        sessCancel,
		}

		sm.mu.Lock()
		sm.sessions[workerSession.ID] = workerSession
		sm.mu.Unlock()

		return c.JSON(http.StatusCreated, StartWorkerSessionResponse{
			WorkerSessionRef:  workerSession.ID,
			Status:            workerSession.Status,
			ContainerID:       workerSession.ContainerID,
			CDPEndpointRef:    workerSession.CDPEndpointRef,
			StreamEndpointRef: workerSession.StreamEndpointRef,
			StartedAt:         time.Now(),
			ExpiresAt:         workerSession.ExpiresAt,
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

	e.POST("/internal/browser-sessions/:ref/capture", func(c echo.Context) error {
		ref := c.Param("ref")
		sm.mu.RLock()
		session, ok := sm.sessions[ref]
		sm.mu.RUnlock()

		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}

		log.Printf("Capture triggered for session %s", ref)

		// Get WebSocket URL using Host spoofing
		var wsURL string
		cdpPort, err := endpointPort(session.CDPEndpointRef)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Invalid CDP endpoint: %v", err))
		}
		reqURL := fmt.Sprintf("http://127.0.0.1:%d/json", cdpPort)

		for i := 0; i < 5; i++ {
			httpReq, _ := http.NewRequest("GET", reqURL, nil)
			httpReq.Host = "localhost" // Bypass Host validation

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(httpReq)
			if err == nil && resp.StatusCode == http.StatusOK {
				var targets []struct {
					Type                 string `json:"type"`
					WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&targets); err == nil {
					for _, t := range targets {
						if t.Type == "page" && t.WebSocketDebuggerUrl != "" {
							// Correct the host in reported URL
							u, _ := url.Parse(t.WebSocketDebuggerUrl)
							u.Host = fmt.Sprintf("127.0.0.1:%d", cdpPort)
							wsURL = u.String()
							break
						}
					}
				}
				resp.Body.Close()
				if wsURL != "" {
					break
				}
			}
			if err != nil {
				log.Printf("Capture check error: %v", err)
			}
			time.Sleep(1 * time.Second)
		}

		if wsURL == "" {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to reach Chromium for capture")
		}

		log.Printf("Capture: Connecting to CDP at %s", wsURL)

		allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), wsURL)
		ctx, cancel := chromedp.NewContext(allocCtx)
		defer cancel()

		var chromeCookies []*network.Cookie
		var username string

		err = chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				chromeCookies, err = network.GetCookies().Do(ctx)
				return err
			}),
			chromedp.ActionFunc(func(ctx context.Context) error {
				script := `(function() {
					const nameEl = document.querySelector('.name-G1vOOn') || 
					               document.querySelector('.user-name') || 
								   document.querySelector('[class*="user-name"]');
					return nameEl ? nameEl.innerText : "";
				})()`
				_ = chromedp.Evaluate(script, &username).Do(ctx)
				return nil
			}),
		)

		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("CDP capture failed: %v", err))
		}

		var cookies []Cookie
		for _, cc := range chromeCookies {
			cookies = append(cookies, Cookie{
				Name: cc.Name, Value: cc.Value, Domain: cc.Domain, Path: cc.Path,
				Expires: cc.Expires, Secure: cc.Secure, HttpOnly: cc.HTTPOnly,
			})
		}

		if ok, _ := validateRequiredCookies(cookies, session.RequiredCookies); !ok {
			return c.JSON(http.StatusOK, CaptureWorkerSessionResponse{
				Status:  "login_not_detected",
				Cookies: cookies,
				Account: RemoteAccountProfile{
					Username: username,
				},
			})
		}

		return c.JSON(http.StatusOK, CaptureWorkerSessionResponse{
			Status:  "login_detected",
			Cookies: cookies,
			Account: RemoteAccountProfile{
				Username: username,
			},
		})
	})

	e.DELETE("/internal/browser-sessions/:ref", func(c echo.Context) error {
		ref := c.Param("ref")
		sm.mu.Lock()
		session, ok := sm.sessions[ref]
		if ok {
			delete(sm.sessions, ref)
		}
		sm.mu.Unlock()

		if ok {
			if session.CancelFunc != nil {
				session.CancelFunc()
			}
			if session.ContainerID != "" {
				dm.StopContainer(context.Background(), session.ContainerID)
			}
		}

		return c.NoContent(http.StatusNoContent)
	})

	e.Logger.Fatal(e.Start(":8081"))
}

func endpointPort(endpoint string) (int, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, err
	}
	var port int
	if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil {
		return 0, fmt.Errorf("missing endpoint port")
	}
	return port, nil
}

func validateRequiredCookies(cookies []Cookie, requirements []CookieRequirement) (bool, []string) {
	var missing []string
	for _, req := range requirements {
		if !req.Required {
			continue
		}
		if !hasRequiredCookie(cookies, req) {
			missing = append(missing, req.Name)
		}
	}
	return len(missing) == 0, missing
}

func hasRequiredCookie(cookies []Cookie, req CookieRequirement) bool {
	for _, cookie := range cookies {
		if cookie.Name != req.Name || cookie.Value == "" {
			continue
		}
		for _, suffix := range req.DomainSuffixes {
			if domainMatches(cookie.Domain, suffix) {
				return true
			}
		}
	}
	return false
}

func domainMatches(domain, suffix string) bool {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	suffix = strings.TrimPrefix(strings.ToLower(suffix), ".")
	return domain == suffix || strings.HasSuffix(domain, "."+suffix)
}
