package observability

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	dbobs "github.com/kurodakayn/mpp-backend/internal/db"
)

const (
	requestIDHeader                   = "X-Request-ID"
	traceIDHeader                     = "X-Trace-ID"
	databaseSlowQueryThresholdEnv     = "DB_SLOW_QUERY_THRESHOLD"
	defaultDatabaseSlowQueryThreshold = 250 * time.Millisecond
)

type contextKey string

const traceIDContextKey contextKey = "trace_id"

type Suite struct {
	serviceName      string
	registry         *prometheus.Registry
	requests         *prometheus.CounterVec
	duration         *prometheus.HistogramVec
	inFlight         *prometheus.GaugeVec
	info             *prometheus.GaugeVec
	databaseObserver *DatabaseQueryObserver
	replicaObserver  *ReplicaLagObserver
	publishObserver  *PublishJobObserver
}

type DatabaseQueryObserver struct {
	serviceName   string
	slowThreshold time.Duration
	queries       *prometheus.CounterVec
	duration      *prometheus.HistogramVec
	slowQueries   *prometheus.CounterVec
}

type ReplicaLagObserver struct {
	serviceName string
	lag         *prometheus.GaugeVec
	healthy     *prometheus.GaugeVec
}

type PublishJobObserver struct {
	serviceName string
	jobs        *prometheus.CounterVec
}

type requestLog struct {
	Time      string  `json:"time"`
	Service   string  `json:"service"`
	TraceID   string  `json:"trace_id"`
	RequestID string  `json:"request_id"`
	Method    string  `json:"method"`
	Path      string  `json:"path"`
	Route     string  `json:"route"`
	Status    int     `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
	RemoteIP  string  `json:"remote_ip"`
	UserAgent string  `json:"user_agent"`
	BytesIn   int64   `json:"bytes_in"`
	BytesOut  int64   `json:"bytes_out"`
	Error     string  `json:"error,omitempty"`
}

type databaseSlowQueryLog struct {
	Time         string  `json:"time"`
	Service      string  `json:"service"`
	TraceID      string  `json:"trace_id,omitempty"`
	Operation    string  `json:"operation"`
	Table        string  `json:"table"`
	Status       string  `json:"status"`
	QueryHash    string  `json:"query_hash"`
	DurationMS   float64 `json:"duration_ms"`
	RowsAffected int64   `json:"rows_affected"`
	SQL          string  `json:"sql"`
	Error        string  `json:"error,omitempty"`
}

func New(serviceName string) *Suite {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "mpp-service"
	}

	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mpp_http_requests_total",
		Help: "Total HTTP requests served by MPP services.",
	}, []string{"service", "method", "route", "status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mpp_http_request_duration_seconds",
		Help:    "HTTP request duration by service, method, route, and status.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"service", "method", "route", "status"})
	inFlight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mpp_http_in_flight_requests",
		Help: "Current in-flight HTTP requests by MPP service.",
	}, []string{"service"})
	info := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mpp_service_info",
		Help: "Static information marker for an MPP service.",
	}, []string{"service"})
	dbQueries := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mpp_db_queries_total",
		Help: "Total database queries executed by MPP services.",
	}, []string{"service", "operation", "table", "status"})
	dbDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mpp_db_query_duration_seconds",
		Help:    "Database query duration by service, operation, table, and status.",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"service", "operation", "table", "status"})
	dbSlowQueries := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mpp_db_slow_queries_total",
		Help: "Total database queries that exceeded the configured slow query threshold.",
	}, []string{"service", "operation", "table", "status"})
	replicaLag := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mpp_db_replica_lag_seconds",
		Help: "Latest observed PostgreSQL read replica lag in seconds.",
	}, []string{"service"})
	replicaHealthy := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mpp_db_replica_healthy",
		Help: "Whether the PostgreSQL read replica is healthy enough for eventual reads.",
	}, []string{"service"})
	publishJobs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mpp_publish_jobs_total",
		Help: "Total publish jobs completed by the publish worker.",
	}, []string{"service", "platform", "result"})

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		requests,
		duration,
		inFlight,
		info,
		dbQueries,
		dbDuration,
		dbSlowQueries,
		replicaLag,
		replicaHealthy,
		publishJobs,
	)
	info.WithLabelValues(serviceName).Set(1)
	databaseObserver := &DatabaseQueryObserver{
		serviceName:   serviceName,
		slowThreshold: databaseSlowQueryThresholdFromEnv(),
		queries:       dbQueries,
		duration:      dbDuration,
		slowQueries:   dbSlowQueries,
	}
	replicaObserver := &ReplicaLagObserver{
		serviceName: serviceName,
		lag:         replicaLag,
		healthy:     replicaHealthy,
	}
	publishObserver := &PublishJobObserver{
		serviceName: serviceName,
		jobs:        publishJobs,
	}

	return &Suite{
		serviceName:      serviceName,
		registry:         registry,
		requests:         requests,
		duration:         duration,
		inFlight:         inFlight,
		info:             info,
		databaseObserver: databaseObserver,
		replicaObserver:  replicaObserver,
		publishObserver:  publishObserver,
	}
}

func (s *Suite) RegisterRoutes(e *echo.Echo) {
	e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{})))
}

func (s *Suite) DatabaseQueryObserver() *DatabaseQueryObserver {
	return s.databaseObserver
}

func (s *Suite) ReplicaLagObserver() *ReplicaLagObserver {
	return s.replicaObserver
}

func (s *Suite) PublishJobObserver() *PublishJobObserver {
	return s.publishObserver
}

func (s *Suite) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			traceID := traceIDFromRequest(req)
			req = req.WithContext(ContextWithTraceID(req.Context(), traceID))
			c.SetRequest(req)
			c.Set("trace_id", traceID)
			c.Set("request_id", traceID)
			c.Response().Header().Set(requestIDHeader, traceID)
			c.Response().Header().Set(traceIDHeader, traceID)

			startedAt := time.Now()
			s.inFlight.WithLabelValues(s.serviceName).Inc()
			err := next(c)
			if err != nil {
				c.Error(err)
			}
			s.inFlight.WithLabelValues(s.serviceName).Dec()

			status := c.Response().Status
			if status == 0 {
				status = http.StatusOK
			}
			route := routePath(c)
			statusLabel := strconv.Itoa(status)
			latency := time.Since(startedAt)
			s.requests.WithLabelValues(s.serviceName, req.Method, route, statusLabel).Inc()
			s.duration.WithLabelValues(s.serviceName, req.Method, route, statusLabel).Observe(latency.Seconds())

			s.logRequest(c, traceID, route, status, latency, err)
			return nil
		}
	}
}

func (o *PublishJobObserver) ObservePublishJob(platform string, result string) {
	if o == nil {
		return
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = "unknown"
	}
	switch result {
	case "success", "error":
	default:
		result = "unknown"
	}
	o.jobs.WithLabelValues(o.serviceName, platform, result).Inc()
}

func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	traceID, _ := ctx.Value(traceIDContextKey).(string)
	return strings.TrimSpace(traceID)
}

func traceIDFromRequest(req *http.Request) string {
	if value := strings.TrimSpace(req.Header.Get(requestIDHeader)); value != "" {
		return value
	}
	if value := strings.TrimSpace(req.Header.Get(traceIDHeader)); value != "" {
		return value
	}
	return uuid.NewString()
}

func routePath(c echo.Context) string {
	if path := strings.TrimSpace(c.Path()); path != "" {
		return path
	}
	if c.Request() != nil && c.Request().URL != nil {
		return c.Request().URL.Path
	}
	return "unknown"
}

func (s *Suite) logRequest(c echo.Context, traceID, route string, status int, latency time.Duration, err error) {
	req := c.Request()
	event := requestLog{
		Time:      time.Now().UTC().Format(time.RFC3339Nano),
		Service:   s.serviceName,
		TraceID:   traceID,
		RequestID: traceID,
		Method:    req.Method,
		Path:      req.URL.Path,
		Route:     route,
		Status:    status,
		LatencyMS: float64(latency.Microseconds()) / 1000,
		RemoteIP:  c.RealIP(),
		UserAgent: req.UserAgent(),
		BytesIn:   req.ContentLength,
		BytesOut:  c.Response().Size,
	}
	if err != nil {
		event.Error = err.Error()
	}
	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		log.Printf("observability request log marshal failed: %v", marshalErr)
		return
	}
	if _, err := os.Stdout.Write(append(payload, '\n')); err != nil {
		log.Printf("observability request log write failed: %v", err)
	}
}

func (o *DatabaseQueryObserver) ObserveQuery(ctx context.Context, observation dbobs.QueryObservation) {
	if o == nil {
		return
	}

	operation := metricLabel(observation.Operation, "unknown")
	table := metricLabel(observation.Table, "unknown")
	status := "ok"
	if observation.Err != nil {
		status = "error"
	}

	durationSeconds := observation.Duration.Seconds()
	if durationSeconds < 0 {
		durationSeconds = 0
	}

	o.queries.WithLabelValues(o.serviceName, operation, table, status).Inc()
	o.duration.WithLabelValues(o.serviceName, operation, table, status).Observe(durationSeconds)

	if o.slowThreshold <= 0 || observation.Duration < o.slowThreshold {
		return
	}

	o.slowQueries.WithLabelValues(o.serviceName, operation, table, status).Inc()
	o.logSlowQuery(ctx, observation, operation, table, status)
}

func (o *DatabaseQueryObserver) logSlowQuery(ctx context.Context, observation dbobs.QueryObservation, operation, table, status string) {
	event := databaseSlowQueryLog{
		Time:         time.Now().UTC().Format(time.RFC3339Nano),
		Service:      o.serviceName,
		TraceID:      TraceIDFromContext(ctx),
		Operation:    operation,
		Table:        table,
		Status:       status,
		QueryHash:    observation.QueryHash,
		DurationMS:   float64(observation.Duration.Microseconds()) / 1000,
		RowsAffected: observation.RowsAffected,
		SQL:          observation.SQL,
	}
	if observation.Err != nil {
		event.Error = observation.Err.Error()
	}

	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		log.Printf("observability database slow query log marshal failed: %v", marshalErr)
		return
	}
	if _, err := os.Stdout.Write(append(payload, '\n')); err != nil {
		log.Printf("observability database slow query log write failed: %v", err)
	}
}

func (o *ReplicaLagObserver) ObserveReplicaLag(_ context.Context, observation dbobs.ReplicaLagObservation) {
	if o == nil {
		return
	}

	lagSeconds := observation.Lag.Seconds()
	if observation.Err != nil || lagSeconds < 0 {
		lagSeconds = -1
	}
	o.lag.WithLabelValues(o.serviceName).Set(lagSeconds)

	healthy := 0.0
	if observation.Healthy {
		healthy = 1
	}
	o.healthy.WithLabelValues(o.serviceName).Set(healthy)
}

func metricLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 96 {
		return value[:96]
	}
	return value
}

func databaseSlowQueryThresholdFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(databaseSlowQueryThresholdEnv))
	if raw == "" {
		return defaultDatabaseSlowQueryThreshold
	}
	threshold, err := time.ParseDuration(raw)
	if err != nil || threshold < 0 {
		return defaultDatabaseSlowQueryThreshold
	}
	return threshold
}
