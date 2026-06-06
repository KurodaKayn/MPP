package observability

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

const unknownRuntimeDriver = "unknown"

type runtimeMetrics struct {
	activeSessions *prometheus.GaugeVec
	starts         *prometheus.CounterVec
	startDuration  *prometheus.HistogramVec
	stops          *prometheus.CounterVec
	cleanupRuns    *prometheus.CounterVec
	ttlExpirations *prometheus.CounterVec
	cleanupLag     *prometheus.GaugeVec
}

func newRuntimeMetrics(registry *prometheus.Registry) *runtimeMetrics {
	metrics := &runtimeMetrics{
		activeSessions: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mpp_browser_runtime_active_sessions",
			Help: "Current browser runtime sessions started by this worker process.",
		}, []string{"driver"}),
		starts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_browser_runtime_session_starts_total",
			Help: "Total browser runtime session start attempts by driver and result.",
		}, []string{"driver", "result"}),
		startDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "mpp_browser_runtime_session_start_duration_seconds",
			Help:    "Browser runtime session start duration by driver and result.",
			Buckets: []float64{0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"driver", "result"}),
		stops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_browser_runtime_session_stops_total",
			Help: "Total browser runtime session stop attempts by driver and result.",
		}, []string{"driver", "result"}),
		cleanupRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_browser_runtime_cleanup_runs_total",
			Help: "Total expired browser runtime cleanup runs by driver and result.",
		}, []string{"driver", "result"}),
		ttlExpirations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mpp_browser_runtime_ttl_expirations_total",
			Help: "Total expired browser runtime sessions deleted by TTL cleanup.",
		}, []string{"driver"}),
		cleanupLag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "mpp_browser_runtime_cleanup_lag_seconds",
			Help: "Observed lag between runtime expiration and TTL cleanup for the oldest runtime cleaned in the latest run.",
		}, []string{"driver"}),
	}
	registry.MustRegister(
		metrics.activeSessions,
		metrics.starts,
		metrics.startDuration,
		metrics.stops,
		metrics.cleanupRuns,
		metrics.ttlExpirations,
		metrics.cleanupLag,
	)
	return metrics
}

func (s *Suite) InstrumentRuntimeManager(manager browserruntime.Manager) browserruntime.Manager {
	if manager == nil {
		return nil
	}
	instrumented := &instrumentedRuntimeManager{
		delegate:       manager,
		metrics:        newRuntimeMetrics(s.registry),
		activeRuntimes: make(map[string]string),
		now:            time.Now,
	}
	if reaper, ok := manager.(browserruntime.ExpiredSessionReaper); ok {
		return &instrumentedReapingRuntimeManager{
			instrumentedRuntimeManager: instrumented,
			reaper:                     reaper,
		}
	}
	return instrumented
}

type instrumentedRuntimeManager struct {
	delegate       browserruntime.Manager
	metrics        *runtimeMetrics
	mu             sync.Mutex
	activeRuntimes map[string]string
	now            func() time.Time
}

func (m *instrumentedRuntimeManager) RuntimeDriver() string {
	return m.delegate.RuntimeDriver()
}

func (m *instrumentedRuntimeManager) StartSession(ctx context.Context, request browserruntime.StartSessionRequest) (browserruntime.SessionReference, error) {
	driver := runtimeDriver(m.delegate)
	startedAt := m.now()
	reference, err := m.delegate.StartSession(ctx, request)
	result := resultLabel(err)
	if reference.Driver != "" {
		driver = reference.Driver
	}
	m.metrics.starts.WithLabelValues(driver, result).Inc()
	m.metrics.startDuration.WithLabelValues(driver, result).Observe(m.now().Sub(startedAt).Seconds())
	if err != nil {
		return reference, err
	}
	m.markActive(reference)
	return reference, nil
}

func (m *instrumentedRuntimeManager) StopSession(ctx context.Context, reference browserruntime.SessionReference) error {
	driver := reference.Driver
	if driver == "" {
		driver = runtimeDriver(m.delegate)
	}
	err := m.delegate.StopSession(ctx, reference)
	m.metrics.stops.WithLabelValues(driver, resultLabel(err)).Inc()
	if err != nil {
		return err
	}
	m.markStopped(reference)
	return nil
}

func (m *instrumentedRuntimeManager) markActive(reference browserruntime.SessionReference) {
	if reference.RuntimeID == "" {
		return
	}
	driver := reference.Driver
	if driver == "" {
		driver = unknownRuntimeDriver
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.activeRuntimes[reference.RuntimeID]; exists {
		return
	}
	m.activeRuntimes[reference.RuntimeID] = driver
	m.metrics.activeSessions.WithLabelValues(driver).Inc()
}

func (m *instrumentedRuntimeManager) markStopped(reference browserruntime.SessionReference) {
	if reference.RuntimeID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	driver, exists := m.activeRuntimes[reference.RuntimeID]
	if !exists {
		return
	}
	delete(m.activeRuntimes, reference.RuntimeID)
	m.metrics.activeSessions.WithLabelValues(driver).Dec()
}

type instrumentedReapingRuntimeManager struct {
	*instrumentedRuntimeManager
	reaper browserruntime.ExpiredSessionReaper
}

func (m *instrumentedReapingRuntimeManager) ReapExpiredSessions(ctx context.Context) (browserruntime.ExpiredSessionReapReport, error) {
	driver := runtimeDriver(m.delegate)
	report, err := m.reaper.ReapExpiredSessions(ctx)
	if report.Driver != "" {
		driver = report.Driver
	}
	m.metrics.cleanupRuns.WithLabelValues(driver, resultLabel(err)).Inc()
	if err != nil {
		return report, err
	}
	m.metrics.ttlExpirations.WithLabelValues(driver).Add(float64(report.DeletedSessions))
	m.metrics.cleanupLag.WithLabelValues(driver).Set(report.CleanupLag(m.now()).Seconds())
	return report, nil
}

func runtimeDriver(manager browserruntime.Manager) string {
	if manager == nil {
		return unknownRuntimeDriver
	}
	driver := manager.RuntimeDriver()
	if driver == "" {
		return unknownRuntimeDriver
	}
	return driver
}

func resultLabel(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}
