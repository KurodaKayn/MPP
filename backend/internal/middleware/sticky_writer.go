package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
)

const (
	StickyWriterCookieName  = "mpp_db_sticky_writer_until"
	StickyWriterHeader      = "X-MPP-DB-Sticky-Writer-Until"
	defaultStickyWriterPath = "/api"
)

const (
	defaultStickyWriterTTL = 10 * time.Second
	maxStickyWriterTTL     = 30 * time.Second
)

type StickyWriterConfig struct {
	TTL    time.Duration
	Path   string
	Now    func() time.Time
	Secure bool
}

func StickyWriter() echo.MiddlewareFunc {
	return StickyWriterWithConfig(StickyWriterConfig{})
}

func StickyWriterWithConfig(config StickyWriterConfig) echo.MiddlewareFunc {
	ttl := config.TTL
	if ttl <= 0 {
		ttl = defaultStickyWriterTTL
	}
	if ttl > maxStickyWriterTTL {
		ttl = maxStickyWriterTTL
	}

	path := strings.TrimSpace(config.Path)
	if path == "" {
		path = defaultStickyWriterPath
	}

	now := config.Now
	if now == nil {
		now = time.Now
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			current := now()
			if until, ok := stickyWriterCookieUntil(c, current); ok {
				req := c.Request()
				req = req.WithContext(dbrouter.WithStickyWriter(req.Context(), until))
				c.SetRequest(req)
			}

			c.Response().Before(func() {
				if shouldRefreshStickyWriter(c.Response().Status, c.Request().Method, c.Request().URL.Path, path) {
					refreshStickyWriter(c, now().Add(ttl), ttl, path, config.Secure)
				}
			})

			return next(c)
		}
	}
}

func stickyWriterCookieUntil(c echo.Context, now time.Time) (time.Time, bool) {
	cookie, err := c.Cookie(StickyWriterCookieName)
	if err != nil {
		return time.Time{}, false
	}
	until, ok := parseStickyWriterUntil(cookie.Value)
	if !ok || !until.After(now) {
		return time.Time{}, false
	}
	if until.After(now.Add(maxStickyWriterTTL)) {
		return time.Time{}, false
	}
	return until, true
}

func parseStickyWriterUntil(raw string) (time.Time, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(value).UTC(), true
}

func shouldRefreshStickyWriter(status int, method string, requestPath string, stickyPath string) bool {
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return false
	}
	if !requestPathMatchesStickyPath(requestPath, stickyPath) {
		return false
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestPathMatchesStickyPath(requestPath string, stickyPath string) bool {
	if stickyPath == "" || stickyPath == "/" {
		return true
	}
	requestPath = "/" + strings.TrimPrefix(requestPath, "/")
	stickyPath = "/" + strings.Trim(strings.TrimSpace(stickyPath), "/")
	return requestPath == stickyPath || strings.HasPrefix(requestPath, stickyPath+"/")
}

func refreshStickyWriter(c echo.Context, until time.Time, ttl time.Duration, path string, secure bool) {
	c.Response().Header().Set(StickyWriterHeader, strconv.FormatInt(until.UnixMilli(), 10))
	// #nosec G124 -- Secure is config-driven so local HTTP development can exercise read-your-write routing.
	c.SetCookie(&http.Cookie{
		Name:     StickyWriterCookieName,
		Value:    strconv.FormatInt(until.UnixMilli(), 10),
		Path:     path,
		Expires:  until,
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
