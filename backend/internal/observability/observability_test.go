package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestMiddlewarePropagatesTraceHeadersAndRecordsMetrics(t *testing.T) {
	e := echo.New()
	suite := New("backend-test")
	suite.RegisterRoutes(e)
	e.Use(suite.Middleware())
	e.GET("/probe/:id", func(c echo.Context) error {
		if got := c.Get("trace_id"); got != "trace-123" {
			t.Fatalf("expected trace id in context, got %v", got)
		}
		return c.String(http.StatusAccepted, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/probe/42", nil)
	req.Header.Set(requestIDHeader, "trace-123")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != "trace-123" {
		t.Fatalf("expected request id response header, got %q", got)
	}
	if got := rec.Header().Get(traceIDHeader); got != "trace-123" {
		t.Fatalf("expected trace id response header, got %q", got)
	}

	metrics := scrapeMetrics(t, e)
	assertMetricLineContains(t, metrics, "mpp_http_requests_total", []string{
		`service="backend-test"`,
		`method="GET"`,
		`route="/probe/:id"`,
		`status="202"`,
	})
	assertMetricLineContains(t, metrics, "mpp_http_request_duration_seconds_count", []string{
		`service="backend-test"`,
		`method="GET"`,
		`route="/probe/:id"`,
		`status="202"`,
	})
}

func TestMiddlewareGeneratesTraceHeaders(t *testing.T) {
	e := echo.New()
	suite := New("backend-test")
	e.Use(suite.Middleware())
	e.GET("/probe", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	requestID := rec.Header().Get(requestIDHeader)
	if requestID == "" {
		t.Fatal("expected generated request id")
	}
	if got := rec.Header().Get(traceIDHeader); got != requestID {
		t.Fatalf("expected trace id to match request id, got %q and %q", got, requestID)
	}
}

func scrapeMetrics(t *testing.T, e *echo.Echo) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", rec.Code)
	}
	return rec.Body.String()
}

func assertMetricLineContains(t *testing.T, metrics, name string, labels []string) {
	t.Helper()

	for _, line := range strings.Split(metrics, "\n") {
		if !strings.HasPrefix(line, name) {
			continue
		}
		matches := true
		for _, label := range labels {
			if !strings.Contains(line, label) {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("expected metric %s with labels %v in:\n%s", name, labels, metrics)
}
