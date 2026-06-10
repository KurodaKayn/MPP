package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReplicaLagMonitorAllowsHealthyReplica(t *testing.T) {
	probe := &fakeReplicaLagProbe{lags: []time.Duration{150 * time.Millisecond}}
	monitor := NewReplicaLagMonitor(probe, ReplicaLagMonitorConfig{
		MaxLag:        time.Second,
		CheckInterval: time.Second,
	})

	require.True(t, monitor.Healthy(context.Background()))
	require.Equal(t, 1, probe.calls)
}

func TestReplicaLagMonitorRejectsLaggedReplica(t *testing.T) {
	probe := &fakeReplicaLagProbe{lags: []time.Duration{2 * time.Second}}
	monitor := NewReplicaLagMonitor(probe, ReplicaLagMonitorConfig{
		MaxLag:        time.Second,
		CheckInterval: time.Second,
	})

	require.False(t, monitor.Healthy(context.Background()))
	require.Equal(t, 1, probe.calls)
}

func TestReplicaLagMonitorRejectsUnknownLag(t *testing.T) {
	probe := &fakeReplicaLagProbe{errors: []error{ErrReplicaLagUnknown}}
	monitor := NewReplicaLagMonitor(probe, ReplicaLagMonitorConfig{
		MaxLag:        time.Second,
		CheckInterval: time.Second,
	})

	require.False(t, monitor.Healthy(context.Background()))
	require.Equal(t, 1, probe.calls)
}

func TestReplicaLagMonitorCachesProbeResultWithinInterval(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	probe := &fakeReplicaLagProbe{
		lags: []time.Duration{
			100 * time.Millisecond,
			3 * time.Second,
		},
	}
	monitor := NewReplicaLagMonitor(probe, ReplicaLagMonitorConfig{
		MaxLag:        time.Second,
		CheckInterval: 5 * time.Second,
		Clock: func() time.Time {
			return now
		},
	})

	require.True(t, monitor.Healthy(context.Background()))
	require.True(t, monitor.Healthy(context.Background()))
	require.Equal(t, 1, probe.calls)

	now = now.Add(6 * time.Second)

	require.False(t, monitor.Healthy(context.Background()))
	require.Equal(t, 2, probe.calls)
}

func TestReplicaLagMonitorDisabledWithoutMaxLag(t *testing.T) {
	probe := &fakeReplicaLagProbe{errors: []error{errors.New("should not be called")}}
	monitor := NewReplicaLagMonitor(probe, ReplicaLagMonitorConfig{})

	require.True(t, monitor.Healthy(context.Background()))
	require.Equal(t, 0, probe.calls)
}

func TestPostgresReplicaLagProbeRejectsNilDatabase(t *testing.T) {
	lag, err := postgresReplicaLagProbe{}.CurrentReplicaLag(context.Background())

	require.Zero(t, lag)
	require.ErrorIs(t, err, ErrReplicaLagUnknown)
}

type fakeReplicaLagProbe struct {
	lags   []time.Duration
	errors []error
	calls  int
}

func (f *fakeReplicaLagProbe) CurrentReplicaLag(context.Context) (time.Duration, error) {
	index := f.calls
	f.calls++

	if index < len(f.errors) && f.errors[index] != nil {
		return 0, f.errors[index]
	}
	if index < len(f.lags) {
		return f.lags[index], nil
	}
	return 0, nil
}
