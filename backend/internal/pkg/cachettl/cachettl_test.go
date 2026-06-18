package cachettl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJitterKeepsTTLWithinLowerBoundAndBase(t *testing.T) {
	base := 15 * time.Second

	ttl := Jitter(base, "mpp:dashboard:projects:list:v2:a")

	require.GreaterOrEqual(t, ttl, 12*time.Second)
	require.LessOrEqual(t, ttl, base)
}

func TestJitterIsDeterministicPerKey(t *testing.T) {
	base := time.Minute

	require.Equal(t, Jitter(base, "same-key"), Jitter(base, "same-key"))
}

func TestJitterCanSpreadDifferentKeys(t *testing.T) {
	base := time.Minute

	values := map[time.Duration]struct{}{}
	for _, key := range []string{"key-a", "key-b", "key-c", "key-d", "key-e"} {
		values[Jitter(base, key)] = struct{}{}
	}

	require.Greater(t, len(values), 1)
}

func TestJitterSkipsInvalidInputs(t *testing.T) {
	require.Equal(t, time.Duration(0), Jitter(0, "key"))
	require.Equal(t, time.Second, JitterPercent(time.Second, "key", 0))
}
