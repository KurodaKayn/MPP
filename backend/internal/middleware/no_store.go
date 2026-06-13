package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const NoStoreCacheControl = "no-store, private"

func NoStoreAPIResponses() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if isAPIPath(c.Request().URL.Path) {
				setNoStoreHeaders(c.Response().Header())
			}
			err := next(c)
			if isAPIPath(c.Request().URL.Path) {
				setNoStoreHeaders(c.Response().Header())
			}
			return err
		}
	}
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func setNoStoreHeaders(headers http.Header) {
	headers.Set(echo.HeaderCacheControl, NoStoreCacheControl)
	headers.Set("Pragma", "no-cache")
	headers.Set("Expires", "0")
}
