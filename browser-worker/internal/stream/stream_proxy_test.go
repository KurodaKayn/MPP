package stream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEndpointPort(t *testing.T) {
	port, err := endpointPort("ws://localhost:49152")
	require.NoError(t, err)
	require.Equal(t, 49152, port)

	_, err = endpointPort("ws://localhost")
	require.Error(t, err)
}
