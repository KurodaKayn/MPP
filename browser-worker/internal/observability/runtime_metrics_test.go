package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

func TestInstrumentRuntimeManagerRecordsStartAndStopMetrics(t *testing.T) {
	suite := New("browser-worker-test")
	manager := &fakeRuntimeManager{driver: browserruntime.DriverKubernetes}
	instrumented := suite.InstrumentRuntimeManager(manager)

	reference, err := instrumented.StartSession(context.Background(), browserruntime.StartSessionRequest{
		SessionID: "session-123",
		UserID:    "user-123",
		Platform:  "douyin",
		TTL:       time.Minute,
	})
	require.NoError(t, err)
	require.NoError(t, instrumented.StopSession(context.Background(), reference))

	metrics := gatherSuiteMetrics(t, suite)
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_session_starts_total", []string{
		`driver="kubernetes"`,
		`result="success"`,
	})
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_session_stops_total", []string{
		`driver="kubernetes"`,
		`result="success"`,
	})
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_active_sessions", []string{
		`driver="kubernetes"`,
		"} 0",
	})
}

func TestInstrumentRuntimeManagerRecordsStartFailures(t *testing.T) {
	suite := New("browser-worker-test")
	manager := &fakeRuntimeManager{
		driver:   browserruntime.DriverDocker,
		startErr: errors.New("start failed"),
	}
	instrumented := suite.InstrumentRuntimeManager(manager)

	_, err := instrumented.StartSession(context.Background(), browserruntime.StartSessionRequest{})

	require.Error(t, err)
	metrics := gatherSuiteMetrics(t, suite)
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_session_starts_total", []string{
		`driver="docker"`,
		`result="error"`,
	})
}

func TestInstrumentRuntimeManagerRecordsCleanupMetrics(t *testing.T) {
	suite := New("browser-worker-test")
	manager := &fakeReapingRuntimeManager{
		fakeRuntimeManager: fakeRuntimeManager{driver: browserruntime.DriverKubernetes},
		report: browserruntime.ExpiredSessionReapReport{
			Driver:          browserruntime.DriverKubernetes,
			DeletedSessions: 2,
			OldestExpiredAt: time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		},
	}
	instrumented, ok := suite.InstrumentRuntimeManager(manager).(browserruntime.ExpiredSessionReaper)
	require.True(t, ok)
	instrumented.(*instrumentedReapingRuntimeManager).now = func() time.Time {
		return time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC)
	}

	report, err := instrumented.ReapExpiredSessions(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, report.DeletedSessions)
	metrics := gatherSuiteMetrics(t, suite)
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_cleanup_runs_total", []string{
		`driver="kubernetes"`,
		`result="success"`,
	})
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_ttl_expirations_total", []string{
		`driver="kubernetes"`,
		"} 2",
	})
	assertMetricLineContains(t, metrics, "mpp_browser_runtime_cleanup_lag_seconds", []string{
		`driver="kubernetes"`,
		"} 300",
	})
}

func gatherSuiteMetrics(t *testing.T, suite *Suite) string {
	t.Helper()

	e := echo.New()
	suite.RegisterRoutes(e)
	return scrapeMetrics(t, e)
}

type fakeRuntimeManager struct {
	driver   string
	startErr error
	stopErr  error
}

func (m *fakeRuntimeManager) RuntimeDriver() string {
	return m.driver
}

func (m *fakeRuntimeManager) StartSession(context.Context, browserruntime.StartSessionRequest) (browserruntime.SessionReference, error) {
	if m.startErr != nil {
		return browserruntime.SessionReference{}, m.startErr
	}
	return browserruntime.SessionReference{
		Driver:    m.driver,
		RuntimeID: "runtime-123",
	}, nil
}

func (m *fakeRuntimeManager) StopSession(context.Context, browserruntime.SessionReference) error {
	return m.stopErr
}

type fakeReapingRuntimeManager struct {
	fakeRuntimeManager
	report browserruntime.ExpiredSessionReapReport
	err    error
}

func (m *fakeReapingRuntimeManager) ReapExpiredSessions(context.Context) (browserruntime.ExpiredSessionReapReport, error) {
	return m.report, m.err
}
