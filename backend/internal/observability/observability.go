package observability

import (
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
)

const (
	requestIDHeader = "X-Request-ID"
	traceIDHeader   = "X-Trace-ID"
)

type Suite struct {
	serviceName string
	registry    *prometheus.Registry
	requests    *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	inFlight    *prometheus.GaugeVec
	info        *prometheus.GaugeVec
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

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		requests,
		duration,
		inFlight,
		info,
	)
	info.WithLabelValues(serviceName).Set(1)

	return &Suite{
		serviceName: serviceName,
		registry:    registry,
		requests:    requests,
		duration:    duration,
		inFlight:    inFlight,
		info:        info,
	}
}

func (s *Suite) RegisterRoutes(e *echo.Echo) {
	e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{})))
}

func (s *Suite) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			traceID := traceIDFromRequest(req)
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
