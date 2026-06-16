package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionReferenceInternalStreamURL(t *testing.T) {
	reference := SessionReference{
		StreamEndpoint: Endpoint{
			Host: "127.0.0.1",
			Port: 6080,
		},
	}

	assert.Equal(t, "http://127.0.0.1:6080", reference.InternalStreamURL())
}

func TestSessionReferenceInternalStreamURLWrapsIPv6Host(t *testing.T) {
	reference := SessionReference{
		StreamEndpoint: Endpoint{
			Host: "::1",
			Port: 6080,
		},
	}

	assert.Equal(t, "http://[::1]:6080", reference.InternalStreamURL())
}

func TestSessionReferenceJSONUsesStableNames(t *testing.T) {
	reference := SessionReference{
		Driver:    DriverDocker,
		RuntimeID: "container-123",
		CDPEndpoint: Endpoint{
			Host: "runtime.local",
			Port: 9222,
		},
		StreamEndpoint: Endpoint{
			Host: "runtime.local",
			Port: 6080,
		},
		CleanupLabels: map[string]string{
			"session_id": "session-123",
		},
	}

	payload, err := json.Marshal(reference)

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"driver": "docker",
		"runtime_id": "container-123",
		"cdp_endpoint": {"host": "runtime.local", "port": 9222},
		"stream_endpoint": {"host": "runtime.local", "port": 6080},
		"cleanup_labels": {"session_id": "session-123"}
	}`, string(payload))
}

func TestExpiredSessionReapReportCleanupLag(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC)

	assert.Equal(t, 5*time.Minute, ExpiredSessionReapReport{
		OldestExpiredAt: now.Add(-5 * time.Minute),
	}.CleanupLag(now))
	assert.Equal(t, time.Duration(0), ExpiredSessionReapReport{}.CleanupLag(now))
	assert.Equal(t, time.Duration(0), ExpiredSessionReapReport{
		OldestExpiredAt: now.Add(time.Minute),
	}.CleanupLag(now))
}
