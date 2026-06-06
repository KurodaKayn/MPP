package runtime

import (
	"encoding/json"
	"testing"

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

func TestSessionReferenceLegacyContainerIDOnlyForDocker(t *testing.T) {
	dockerReference := SessionReference{
		Driver:    DriverDocker,
		RuntimeID: "container-123",
	}
	kubernetesReference := SessionReference{
		Driver:    DriverKubernetes,
		RuntimeID: "pod-123",
	}

	assert.Equal(t, "container-123", dockerReference.LegacyContainerID())
	assert.Empty(t, kubernetesReference.LegacyContainerID())
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
