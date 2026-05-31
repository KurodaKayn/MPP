package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEndpointPort(t *testing.T) {
	port, err := endpointPort("ws://localhost:49152")
	assert.NoError(t, err)
	assert.Equal(t, 49152, port)

	_, err = endpointPort("ws://localhost")
	assert.Error(t, err)
}
