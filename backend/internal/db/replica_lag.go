package db

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"
)

const postgresReplicaLagSecondsQuery = `
SELECT CASE
	WHEN NOT pg_is_in_recovery() THEN 0
	WHEN NOT EXISTS (
		SELECT 1
		FROM pg_stat_wal_receiver
	) THEN NULL
	WHEN pg_last_wal_receive_lsn() IS NULL OR pg_last_wal_replay_lsn() IS NULL THEN NULL
	WHEN pg_last_wal_receive_lsn() = pg_last_wal_replay_lsn() THEN 0
	WHEN pg_last_xact_replay_timestamp() IS NULL THEN NULL
	ELSE EXTRACT(EPOCH FROM now() - pg_last_xact_replay_timestamp())
END
`

var ErrReplicaLagUnknown = errors.New("database replica lag is unknown")

type ReplicaLagChecker interface {
	Healthy(context.Context) bool
}

type ReplicaLagProbe interface {
	CurrentReplicaLag(context.Context) (time.Duration, error)
}

type ReplicaLagObservation struct {
	Lag     time.Duration
	MaxLag  time.Duration
	Healthy bool
	Err     error
}

type ReplicaLagObserver interface {
	ObserveReplicaLag(context.Context, ReplicaLagObservation)
}

type ReplicaLagMonitorConfig struct {
	MaxLag        time.Duration
	CheckInterval time.Duration
	Clock         func() time.Time
}

type ReplicaLagMonitor struct {
	probe         ReplicaLagProbe
	maxLag        time.Duration
	checkInterval time.Duration
	clock         func() time.Time

	mu        sync.Mutex
	checkedAt time.Time
	healthy   bool
	observer  ReplicaLagObserver
}

func NewReplicaLagMonitor(probe ReplicaLagProbe, config ReplicaLagMonitorConfig) *ReplicaLagMonitor {
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}

	return &ReplicaLagMonitor{
		probe:         probe,
		maxLag:        config.MaxLag,
		checkInterval: config.CheckInterval,
		clock:         clock,
		healthy:       true,
	}
}

func NewPostgresReplicaLagMonitor(reader *gorm.DB, config ReplicaLagMonitorConfig) *ReplicaLagMonitor {
	if reader == nil {
		return nil
	}
	return NewReplicaLagMonitor(postgresReplicaLagProbe{database: reader}, config)
}

func (m *ReplicaLagMonitor) Healthy(ctx context.Context) bool {
	if m == nil || m.probe == nil || m.maxLag <= 0 {
		return true
	}

	now := m.clock()

	m.mu.Lock()
	if !m.checkedAt.IsZero() && m.checkInterval > 0 && now.Sub(m.checkedAt) < m.checkInterval {
		healthy := m.healthy
		m.mu.Unlock()
		return healthy
	}

	lag, err := m.probe.CurrentReplicaLag(ctx)
	m.checkedAt = now
	m.healthy = err == nil && lag <= m.maxLag
	healthy := m.healthy
	observer := m.observer
	maxLag := m.maxLag
	m.mu.Unlock()

	if observer != nil {
		observer.ObserveReplicaLag(ctx, ReplicaLagObservation{
			Lag:     lag,
			MaxLag:  maxLag,
			Healthy: healthy,
			Err:     err,
		})
	}
	return healthy
}

func (m *ReplicaLagMonitor) SetReplicaLagObserver(observer ReplicaLagObserver) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observer = observer
}

type postgresReplicaLagProbe struct {
	database *gorm.DB
}

func (p postgresReplicaLagProbe) CurrentReplicaLag(ctx context.Context) (time.Duration, error) {
	if p.database == nil {
		return 0, ErrReplicaLagUnknown
	}
	if ctx == nil {
		ctx = context.Background()
	}

	row := p.database.WithContext(ctx).Raw(postgresReplicaLagSecondsQuery).Row()
	var lagSeconds sql.NullFloat64
	if err := row.Scan(&lagSeconds); err != nil {
		return 0, err
	}
	if !lagSeconds.Valid {
		return 0, ErrReplicaLagUnknown
	}
	if lagSeconds.Float64 < 0 {
		return 0, nil
	}
	return time.Duration(lagSeconds.Float64 * float64(time.Second)), nil
}
