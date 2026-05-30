package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
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
	ContainerID       string
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

		// 1. Start Docker Container
		containerID, _, cdpPort, streamPort, err := dm.StartBrowserContainer(c.Request().Context(), req.SessionID.String())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to start browser: %v", err))
		}

		// 2. Setup Security Interception (CDP)
		go func() {
			var ctx context.Context
			var cancel func()
			
			// Wait for Chromium to write the DevToolsActivePort file
			time.Sleep(3 * time.Second)

			var browserUUID string
			for i := 0; i < 5; i++ {
				uuid, err := dm.GetBrowserUUID(context.Background(), containerID)
				if err == nil && uuid != "" {
					browserUUID = uuid
					break
				}
				time.Sleep(1 * time.Second)
			}

			if browserUUID == "" {
				log.Printf("CRITICAL: Failed to read browser UUID from container")
				return
			}

			// Construct the exact websocket URL to bypass Chromium's HTTP Host check on /json/version
			wsURL := fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser/%s", cdpPort, browserUUID)
			log.Printf("Attempting direct CDP connection to: %s", wsURL)

			// Retry loop for CDP connection
			for i := 0; i < 5; i++ {
				// Use direct websocket connection instead of RemoteAllocator HTTP fetch
				allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), wsURL)
				
				// Find existing targets to avoid creating a new hidden tab
				infos, err := chromedp.Targets(allocCtx)
				if err == nil && len(infos) > 0 {
					var targetID string
					for _, info := range infos {
						if info.Type == "page" {
							targetID = string(info.TargetID)
							break
						}
					}
					if targetID != "" {
						ctx, cancel = chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetID)))
					} else {
						ctx, cancel = chromedp.NewContext(allocCtx)
					}
				} else {
					ctx, cancel = chromedp.NewContext(allocCtx)
				}
				
				// Attempt a simple run to test connection
				err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
					return nil
				}))
				
				if err == nil {
					log.Printf("Successfully connected to CDP at %s", wsURL)
					break
				}
				
				log.Printf("Waiting for CDP at %s (attempt %d/5)... error: %v", wsURL, i+1, err)
				cancel()
				if i == 4 {
					log.Printf("CRITICAL: Failed to connect to CDP after 5 attempts")
					return
				}
				time.Sleep(2 * time.Second)
			}

			if err := SetupInterception(ctx, req.AllowedDomains); err != nil {
				log.Printf("Failed to setup interception for %s: %v", wsURL, err)
			}
			
			chromedp.Run(ctx, chromedp.Navigate(req.LoginURL))
		}()

		ref := uuid.NewString()
		now := time.Now()
		expiresAt := now.Add(time.Duration(req.TTLSeconds) * time.Second)

		// Note: In Docker Desktop on Windows/Mac, localhost:port is how you reach the mapped port.
		// In production Linux, it might be the host IP.
		session := &WorkerSession{
			ID:                ref,
			SessionID:         req.SessionID,
			UserID:            req.UserID,
			Platform:          req.Platform,
			Status:            "ready",
			ContainerID:       containerID,
			CDPEndpointRef:    fmt.Sprintf("ws://localhost:%d", cdpPort),
			StreamEndpointRef: fmt.Sprintf("http://localhost:%d", streamPort),
			ExpiresAt:         expiresAt,
		}

		sm.mu.Lock()
		sm.sessions[ref] = session
		sm.mu.Unlock()

		return c.JSON(http.StatusCreated, StartWorkerSessionResponse{
			WorkerSessionRef:  ref,
			Status:            session.Status,
			ContainerID:       session.ContainerID,
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

	e.POST("/internal/browser-sessions/:ref/capture", func(c echo.Context) error {
		ref := c.Param("ref")
		sm.mu.RLock()
		session, ok := sm.sessions[ref]
		sm.mu.RUnlock()

		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}

		// Connect to the remote browser
		allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), session.CDPEndpointRef)
		ctx, cancel := chromedp.NewContext(allocCtx)
		defer cancel()

		var chromeCookies []*network.Cookie
		var username string
		
		err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				chromeCookies, err = network.GetCookies().Do(ctx)
				return err
			}),
			// Best-effort account extraction (can be platform specific later)
			chromedp.Evaluate(`(function() {
				const nameEl = document.querySelector('.user-name') || document.querySelector('[class*="user-name"]');
				return nameEl ? nameEl.innerText : "";
			})()`, &username),
		)

		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("CDP capture failed: %v", err))
		}

		// Map cookies
		var cookies []Cookie
		for _, cc := range chromeCookies {
			cookies = append(cookies, Cookie{
				Name: cc.Name, Value: cc.Value, Domain: cc.Domain, Path: cc.Path,
				Expires: cc.Expires, Secure: cc.Secure, HttpOnly: cc.HTTPOnly,
			})
		}

		return c.JSON(http.StatusOK, CaptureWorkerSessionResponse{
			Status:  "login_detected", // In a real app, we'd validate requirements here
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

		if ok && session.ContainerID != "" {
			dm.StopContainer(context.Background(), session.ContainerID)
		}

		return c.NoContent(http.StatusNoContent)
	})

	e.Logger.Fatal(e.Start(":8081"))
}
