package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	cdpproto "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-browser-worker/internal/cdp"
	browsercontainer "github.com/kurodakayn/mpp-browser-worker/internal/container"
	"github.com/kurodakayn/mpp-browser-worker/internal/cookies"
	"github.com/kurodakayn/mpp-browser-worker/internal/isolation"
	workerpublish "github.com/kurodakayn/mpp-browser-worker/internal/publish"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
	"github.com/kurodakayn/mpp-browser-worker/internal/sessionstate"
	"github.com/kurodakayn/mpp-browser-worker/internal/stream"
	"github.com/labstack/echo/v4"
)

type Server struct {
	containers *browsercontainer.Manager
	sessions   *session.Manager
	stateStore *session.RedisStateStore
}

func New(containers *browsercontainer.Manager, sessions *session.Manager, stateStore *session.RedisStateStore) *Server {
	return &Server{
		containers: containers,
		sessions:   sessions,
		stateStore: stateStore,
	}
}

func (s *Server) RegisterRoutes(e *echo.Echo) {
	e.POST("/internal/browser-sessions", s.createSession)
	e.GET("/internal/browser-sessions/:ref", s.getSession)
	e.Any("/internal/browser-sessions/:ref/stream", stream.Handler(s.sessions))
	e.Any("/internal/browser-sessions/:ref/stream/*", stream.Handler(s.sessions))
	e.POST("/internal/browser-sessions/:ref/capture", s.captureSession)
	e.POST("/internal/browser-sessions/:ref/publish/douyin", s.startDouyinPublish)
	e.DELETE("/internal/browser-sessions/:ref", s.deleteSession)
}

func (s *Server) ShutdownSessions(ctx context.Context) {
	for _, workerSession := range s.sessions.List() {
		if removedSession, ok := s.sessions.Remove(workerSession.ID); ok {
			cleanupSession(ctx, s.containers, removedSession)
		}
	}
}

func (s *Server) createSession(c echo.Context) error {
	var req session.StartWorkerSessionRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Start at about:blank so request interception is active before platform navigation.
	containerID, containerHost, cdpPort, streamPort, err := s.containers.StartBrowserContainer(c.Request().Context(), req.SessionID.String())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to start browser: %v", err))
	}

	wsURL, err := cdp.VersionWebSocketURL(containerHost, cdpPort)
	if err != nil {
		_ = s.containers.StopContainer(context.Background(), containerID)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to obtain WebSocket URL for security configuration")
	}

	log.Printf("Session %s: Connecting to CDP at %s", req.SessionID, wsURL)

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	pageTargetID, err := cdp.PageTargetID(containerHost, cdpPort)
	if err != nil {
		allocCancel()
		_ = s.containers.StopContainer(context.Background(), containerID)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to find browser page target")
	}
	browserCtx, browserCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(pageTargetID))
	if err := isolation.SetupInterception(browserCtx, req.AllowedDomains); err != nil {
		browserCancel()
		allocCancel()
		_ = s.containers.StopContainer(context.Background(), containerID)
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to configure browser isolation: %v", err))
	}
	if len(req.InitialCookies) > 0 {
		if err := chromedp.Run(browserCtx, restoreCookiesAction(req.InitialCookies)); err != nil {
			browserCancel()
			allocCancel()
			_ = s.containers.StopContainer(context.Background(), containerID)
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to restore browser cookies: %v", err))
		}
	}
	if err := chromedp.Run(browserCtx, chromedp.Navigate(req.LoginURL)); err != nil {
		browserCancel()
		allocCancel()
		_ = s.containers.StopContainer(context.Background(), containerID)
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to navigate to login page: %v", err))
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	startedAt := time.Now()
	expiresAt := startedAt.Add(ttl)
	ref := uuid.NewString()

	workerSession := &session.WorkerSession{
		ID:                ref,
		SessionID:         req.SessionID,
		UserID:            req.UserID,
		Platform:          req.Platform,
		Status:            "ready",
		ContainerID:       containerID,
		CDPEndpointRef:    "internal-cdp:" + ref,
		StreamEndpointRef: "",
		InternalStreamURL: fmt.Sprintf("http://%s:%d", containerHost, streamPort),
		RequiredCookies:   req.RequiredCookies,
		CreatedAt:         startedAt,
		ExpiresAt:         expiresAt,
		BrowserContext:    browserCtx,
		StateStore:        s.stateStore,
		CancelFunc: func() {
			browserCancel()
			allocCancel()
		},
	}
	workerSession.StreamEndpointRef = fmt.Sprintf("/internal/browser-sessions/%s/stream", workerSession.ID)
	workerSession.StateCancel = sessionstate.StartLoop(context.Background(), workerSession)

	s.sessions.Put(workerSession)
	time.AfterFunc(ttl, func() {
		removedSession, ok := s.sessions.Remove(workerSession.ID)
		if !ok {
			return
		}
		cleanupSession(context.Background(), s.containers, removedSession)
	})

	return c.JSON(http.StatusCreated, session.StartWorkerSessionResponse{
		WorkerSessionRef:  workerSession.ID,
		Status:            workerSession.Status,
		ContainerID:       workerSession.ContainerID,
		CDPEndpointRef:    workerSession.CDPEndpointRef,
		StreamEndpointRef: workerSession.StreamEndpointRef,
		StartedAt:         startedAt,
		ExpiresAt:         workerSession.ExpiresAt,
	})
}

func (s *Server) getSession(c echo.Context) error {
	ref := c.Param("ref")
	workerSession, ok := s.sessions.Get(ref)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	state, err := sessionstate.DetectAndSave(c.Request().Context(), workerSession)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, state)
}

func (s *Server) captureSession(c echo.Context) error {
	ref := c.Param("ref")
	workerSession, ok := s.sessions.Get(ref)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	log.Printf("Capture triggered for session %s (container %s)", ref, workerSession.ContainerID)

	currentURL, browserCookies, username, err := cdp.Snapshot(c.Request().Context(), workerSession, true)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("CDP capture failed: %v", err))
	}

	preservedCookies := cookies.FilterPreserved(browserCookies, workerSession.RequiredCookies)
	ok, missing := cookies.ValidateRequired(preservedCookies, workerSession.RequiredCookies)
	if !ok {
		state := session.WorkerSessionState{
			WorkerSessionRef: ref,
			Status:           "ready",
			CurrentURL:       currentURL,
			MissingCookies:   missing,
			Message:          "Waiting for required login cookies",
			ExpiresAt:        workerSession.ExpiresAt,
		}
		workerSession.Status = state.Status
		_ = workerSession.StateStore.SaveLiveSession(c.Request().Context(), workerSession, state)
		return c.JSON(http.StatusOK, session.CaptureWorkerSessionResponse{
			Status:         "ready",
			MissingCookies: missing,
		})
	}
	if username == "" {
		username = cookies.DefaultAccountUsername(workerSession.Platform)
	}

	state := session.WorkerSessionState{
		WorkerSessionRef: ref,
		Status:           "login_detected",
		CurrentURL:       currentURL,
		LoginDetected:    true,
		Message:          "Login detected successfully",
		ExpiresAt:        workerSession.ExpiresAt,
	}
	workerSession.Status = state.Status
	_ = workerSession.StateStore.SaveLiveSession(c.Request().Context(), workerSession, state)
	return c.JSON(http.StatusOK, session.CaptureWorkerSessionResponse{
		Status:  state.Status,
		Cookies: preservedCookies,
		Account: session.RemoteAccountProfile{
			Username: username,
		},
	})
}

func (s *Server) startDouyinPublish(c echo.Context) error {
	ref := c.Param("ref")
	workerSession, ok := s.sessions.Get(ref)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if workerSession.Platform != "douyin" {
		return echo.NewHTTPError(http.StatusBadRequest, "session platform is not douyin")
	}

	var req workerpublish.DouyinDraftRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	go func() {
		if err := workerpublish.RunDouyinDraft(context.Background(), workerSession, req); err != nil {
			log.Printf("Douyin publish script failed for session %s: %v", ref, err)
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]string{
		"status":  "started",
		"message": "douyin publish script started",
	})
}

func (s *Server) deleteSession(c echo.Context) error {
	ref := c.Param("ref")
	workerSession, ok := s.sessions.Remove(ref)
	if ok {
		cleanupSession(context.Background(), s.containers, workerSession)
	}
	return c.NoContent(http.StatusNoContent)
}

func cleanupSession(ctx context.Context, containers *browsercontainer.Manager, workerSession *session.WorkerSession) {
	if workerSession.StateCancel != nil {
		workerSession.StateCancel()
	}
	_ = workerSession.StateStore.DeleteHeartbeat(ctx, workerSession.ID)
	if workerSession.CancelFunc != nil {
		workerSession.CancelFunc()
	}
	if workerSession.ContainerID != "" {
		if err := containers.StopContainer(ctx, workerSession.ContainerID); err != nil {
			log.Printf("Failed to stop session container %s: %v", workerSession.ContainerID, err)
		}
	}
}

func restoreCookiesAction(cookies []session.Cookie) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		for _, cookie := range cookies {
			expr := network.SetCookie(cookie.Name, cookie.Value).
				WithDomain(cookie.Domain).
				WithPath(cookie.Path).
				WithHTTPOnly(cookie.HttpOnly).
				WithSecure(cookie.Secure)

			if cookie.Expires > 0 {
				expires := cdpproto.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))
				expr = expr.WithExpires(&expires)
			}
			if cookie.SameSite != "" {
				expr = expr.WithSameSite(network.CookieSameSite(cookie.SameSite))
			}

			if err := expr.Do(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
