package container

import (
	"testing"

	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerNetworkIP(t *testing.T) {
	host, err := containerNetworkIP(map[string]*network.EndpointSettings{
		"runtime": {IPAddress: "172.20.0.8"},
	}, "runtime")

	require.NoError(t, err)
	assert.Equal(t, "172.20.0.8", host)
}

func TestContainerNetworkIPMissingNetwork(t *testing.T) {
	_, err := containerNetworkIP(map[string]*network.EndpointSettings{}, "runtime")

	assert.Error(t, err)
}
