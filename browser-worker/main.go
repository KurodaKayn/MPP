package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
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
	InternalStreamURL string
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

func (sm *SessionManager) get(ref string) (*WorkerSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[ref]
	return session, ok
}

func (sm *SessionManager) put(session *WorkerSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[session.ID] = session
}

func (sm *SessionManager) remove(ref string) (*WorkerSession, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	session, ok := sm.sessions[ref]
	if ok {
		delete(sm.sessions, ref)
	}
	return session, ok
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

		// 2. Obtain WebSocket URL via manual HTTP fetch with Host spoofing
		var wsURL string
		for i := 0; i < 10; i++ {
			reqURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", cdpPort)
			httpReq, _ := http.NewRequest("GET", reqURL, nil)
			httpReq.Host = "localhost" // Bypass Chromium Host check

			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(httpReq)
			if err == nil && resp.StatusCode == http.StatusOK {
				var result struct {
					WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.WebSocketDebuggerUrl != "" {
					u, _ := url.Parse(result.WebSocketDebuggerUrl)
					u.Host = fmt.Sprintf("127.0.0.1:%d", cdpPort)
					wsURL = u.String()
					resp.Body.Close()
					break
				}
				resp.Body.Close()
			}
			time.Sleep(1 * time.Second)
		}

		if wsURL == "" {
			_ = dm.StopContainer(context.Background(), containerID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to obtain WebSocket URL for security configuration")
		}

		log.Printf("Session %s: Connecting to CDP at %s", req.SessionID, wsURL)

		allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
		browserCtx, browserCancel := chromedp.NewContext(allocCtx)
		_ = browserCtx
		/*
		if err := SetupInterception(browserCtx, req.AllowedDomains); err != nil {
			browserCancel()
			allocCancel()
			_ = dm.StopContainer(context.Background(), containerID)
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to configure browser isolation: %v", err))
		}
		*/

		ttl := time.Duration(req.TTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 15 * time.Minute
		}
		expiresAt := time.Now().Add(ttl)

		workerSession := &WorkerSession{
			ID:                uuid.NewString(),
			SessionID:         req.SessionID,
			UserID:            req.UserID,
			Platform:          req.Platform,
			Status:            "ready",
			ContainerID:       containerID,
			CDPEndpointRef:    fmt.Sprintf("ws://localhost:%d", cdpPort),
			StreamEndpointRef: "",
			InternalStreamURL: fmt.Sprintf("http://127.0.0.1:%d", streamPort),
			RequiredCookies:   req.RequiredCookies,
			ExpiresAt:         expiresAt,
			CancelFunc: func() {
				browserCancel()
				allocCancel()
			},
		}
		workerSession.StreamEndpointRef = fmt.Sprintf("/internal/browser-sessions/%s/stream", workerSession.ID)

		sm.put(workerSession)
		time.AfterFunc(ttl, func() {
			session, ok := sm.remove(workerSession.ID)
			if !ok {
				return
			}
			cleanupSession(context.Background(), dm, session)
		})

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
		session, ok := sm.get(ref)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"worker_session_ref": ref,
			"status":             session.Status,
			"expires_at":         session.ExpiresAt,
		})
	})

	e.Any("/internal/browser-sessions/:ref/stream", browserStreamHandler(sm, dm))
	e.Any("/internal/browser-sessions/:ref/stream/*", browserStreamHandler(sm, dm))

	e.POST("/internal/browser-sessions/:ref/capture", func(c echo.Context) error {
		ref := c.Param("ref")
		session, ok := sm.get(ref)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}

		log.Printf("Capture triggered for session %s (container %s)", ref, session.ContainerID)

		// 1. Obtain current WebSocket URL via manual HTTP fetch
		cdpPort := 9222 
		wsURL, err := browserWebSocketURL(cdpPort)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to reach Chromium for capture: %v", err))
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
				// Use storage.GetCookies to get ALL cookies from the browser instance
				allCookies, err := storage.GetCookies().Do(ctx)
				if err != nil {
					return err
				}
				// Map storage.Cookie to network.Cookie (compatible fields)
				for _, sc := range allCookies {
					chromeCookies = append(chromeCookies, &network.Cookie{
						Name:     sc.Name,
						Value:    sc.Value,
						Domain:   sc.Domain,
						Path:     sc.Path,
						Expires:  sc.Expires,
						Size:     sc.Size,
						HTTPOnly: sc.HTTPOnly,
						Secure:   sc.Secure,
						Session:  sc.Session,
					})
				}
				return nil
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

		// Force 'login_detected' as requested, since calling complete implies login success
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
		session, ok := sm.remove(ref)
		if ok {
			cleanupSession(context.Background(), dm, session)
		}
		return c.NoContent(http.StatusNoContent)
	})

	e.Logger.Fatal(e.Start(":8081"))
}

func browserStreamHandler(sm *SessionManager, dm *DockerManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := c.Param("ref")
		session, ok := sm.get(ref)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}

		targetURL, err := url.Parse(session.InternalStreamURL)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "invalid stream endpoint")
		}

		if strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket" {
			subPath := c.Param("*")
			if subPath != "" {
				if !strings.HasPrefix(subPath, "/") {
					subPath = "/" + subPath
				}
				targetURL.Path = subPath
			}
			return proxyWebSocket(c, targetURL)
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.Director = func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			subPath := c.Param("*")
			if subPath == "" {
				req.URL.Path = "/"
			} else {
				if !strings.HasPrefix(subPath, "/") {
					subPath = "/" + subPath
				}
				req.URL.Path = subPath
			}
			req.URL.RawQuery = c.Request().URL.RawQuery
			req.Host = targetURL.Host
		}
		proxy.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

func cleanupSession(ctx context.Context, dm *DockerManager, session *WorkerSession) {
	if session.CancelFunc != nil {
		session.CancelFunc()
	}
	if session.ContainerID != "" {
		if err := dm.StopContainer(ctx, session.ContainerID); err != nil {
			log.Printf("Failed to stop session container %s: %v", session.ContainerID, err)
		}
	}
}

func proxyWebSocket(c echo.Context, target *url.URL) error {
	req := c.Request()
	res := c.Response()

	targetAddr := target.Host
	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":80"
	}

	d := net.Dialer{}
	targetConn, err := d.DialContext(req.Context(), "tcp", targetAddr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to connect to stream target")
	}
	defer targetConn.Close()

	targetReq, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), nil)
	if err != nil {
		return err
	}
	for k, vv := range req.Header {
		for _, v := range vv {
			targetReq.Header.Add(k, v)
		}
	}
	targetReq.Host = target.Host

	if err := targetReq.Write(targetConn); err != nil {
		return err
	}

	hijacker, ok := res.Writer.(http.Hijacker)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "webserver does not support hijacking")
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return err
	}
	defer clientConn.Close()

	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errChan <- err
	}

	go cp(targetConn, clientConn)
	go cp(clientConn, targetConn)

	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case err := <-errChan:
		if err != nil && err != io.EOF {
			log.Printf("WebSocket worker proxy error: %v", err)
		}
		return nil
	}
}

func browserWebSocketURL(cdpPort int) (string, error) {
	reqURL := fmt.Sprintf("http://127.0.0.1:%d/json", cdpPort)
	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < 5; i++ {
		httpReq, _ := http.NewRequest(http.MethodGet, reqURL, nil)
		httpReq.Host = "localhost" // Bypass Host validation

		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("CDP target check error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}

			var targets []struct {
				Type                 string `json:"type"`
				WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
				return
			}
			for _, t := range targets {
				if t.Type == "page" && t.WebSocketDebuggerUrl != "" {
					u, _ := url.Parse(t.WebSocketDebuggerUrl)
					u.Host = fmt.Sprintf("127.0.0.1:%d", cdpPort)
					reqURL = u.String()
					break
				}
			}
		}()

		if strings.HasPrefix(reqURL, "ws://") {
			return reqURL, nil
		}
		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("no page websocket target found on CDP port %d", cdpPort)
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

func workerStreamPath(wildcardPath string) string {
	wildcardPath = strings.TrimPrefix(wildcardPath, "/")
	if wildcardPath == "" {
		return "/"
	}
	return "/" + wildcardPath
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
