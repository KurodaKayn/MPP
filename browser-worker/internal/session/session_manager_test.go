package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerReservationsCountTowardPoolLimit(t *testing.T) {
	manager := NewManagerWithLimit(1)

	require.True(t, manager.TryReserve())
	assert.False(t, manager.TryReserve())

	manager.ReleaseReservation()
	assert.True(t, manager.TryReserve())
}

func TestManagerActiveSessionsCountTowardPoolLimit(t *testing.T) {
	manager := NewManagerWithLimit(1)

	require.True(t, manager.TryReserve())
	manager.Put(&WorkerSession{ID: "session-1"})
	manager.ReleaseReservation()

	assert.False(t, manager.TryReserve())

	removed, ok := manager.Remove("session-1")
	require.True(t, ok)
	assert.Equal(t, "session-1", removed.ID)
	assert.True(t, manager.TryReserve())
}

func TestManagerZeroLimitIsUnlimited(t *testing.T) {
	manager := NewManagerWithLimit(0)

	assert.True(t, manager.TryReserve())
	assert.True(t, manager.TryReserve())
	assert.True(t, manager.TryReserve())
}
