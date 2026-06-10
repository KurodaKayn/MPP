package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	dbobs "github.com/kurodakayn/mpp-backend/internal/db"
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

func TestDatabaseObserverRecordsQueryMetricsAndSlowQueries(t *testing.T) {
	t.Setenv(databaseSlowQueryThresholdEnv, "1ms")
	e := echo.New()
	suite := New("backend-test")
	suite.RegisterRoutes(e)

	suite.DatabaseQueryObserver().ObserveQuery(
		ContextWithTraceID(context.Background(), "trace-123"),
		dbobs.QueryObservation{
			Operation:    "query",
			Table:        "projects",
			SQL:          "SELECT id FROM projects WHERE user_id = ?",
			QueryHash:    "abc123",
			Duration:     2 * time.Millisecond,
			RowsAffected: 1,
		},
	)

	metrics := scrapeMetrics(t, e)
	assertMetricLineContains(t, metrics, "mpp_db_queries_total", []string{
		`service="backend-test"`,
		`operation="query"`,
		`table="projects"`,
		`status="ok"`,
	})
	assertMetricLineContains(t, metrics, "mpp_db_query_duration_seconds_count", []string{
		`service="backend-test"`,
		`operation="query"`,
		`table="projects"`,
		`status="ok"`,
	})
	assertMetricLineContains(t, metrics, "mpp_db_slow_queries_total", []string{
		`service="backend-test"`,
		`operation="query"`,
		`table="projects"`,
		`status="ok"`,
	})
}

func TestReplicaLagObserverRecordsReplicaMetrics(t *testing.T) {
	e := echo.New()
	suite := New("backend-test")
	suite.RegisterRoutes(e)

	suite.ReplicaLagObserver().ObserveReplicaLag(
		context.Background(),
		dbobs.ReplicaLagObservation{
			Lag:     1500 * time.Millisecond,
			MaxLag:  2 * time.Second,
			Healthy: true,
		},
	)

	metrics := scrapeMetrics(t, e)
	assertMetricLineContains(t, metrics, "mpp_db_replica_lag_seconds", []string{
		`service="backend-test"`,
		"} 1.5",
	})
	assertMetricLineContains(t, metrics, "mpp_db_replica_healthy", []string{
		`service="backend-test"`,
		"} 1",
	})
}

func TestReplicaLagObserverRecordsUnhealthyUnknownLag(t *testing.T) {
	e := echo.New()
	suite := New("backend-test")
	suite.RegisterRoutes(e)

	suite.ReplicaLagObserver().ObserveReplicaLag(
		context.Background(),
		dbobs.ReplicaLagObservation{
			Err: dbobs.ErrReplicaLagUnknown,
		},
	)

	metrics := scrapeMetrics(t, e)
	assertMetricLineContains(t, metrics, "mpp_db_replica_lag_seconds", []string{
		`service="backend-test"`,
		"} -1",
	})
	assertMetricLineContains(t, metrics, "mpp_db_replica_healthy", []string{
		`service="backend-test"`,
		"} 0",
	})
}

func TestPublishJobObserverRecordsPublishMetrics(t *testing.T) {
	e := echo.New()
	suite := New("publish-worker-test")
	suite.RegisterRoutes(e)

	suite.PublishJobObserver().ObservePublishJob("wechat", "error")

	metrics := scrapeMetrics(t, e)
	assertMetricLineContains(t, metrics, "mpp_publish_jobs_total", []string{
		`service="publish-worker-test"`,
		`platform="wechat"`,
		`result="error"`,
	})
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

	for line := range strings.SplitSeq(metrics, "\n") {
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
