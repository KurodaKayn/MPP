package runtimefactory

import (
	"testing"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriverFromEnvDefaultsToDocker(t *testing.T) {
	t.Setenv(runtimeDriverEnv, "")

	driver, err := DriverFromEnv()

	require.NoError(t, err)
	assert.Equal(t, browserruntime.DriverDocker, driver)
}

func TestDriverFromEnvNormalizesConfiguredDriver(t *testing.T) {
	t.Setenv(runtimeDriverEnv, " Kubernetes ")

	driver, err := DriverFromEnv()

	require.NoError(t, err)
	assert.Equal(t, browserruntime.DriverKubernetes, driver)
}

func TestDriverFromEnvRejectsUnsupportedDriver(t *testing.T) {
	t.Setenv(runtimeDriverEnv, "podman")

	driver, err := DriverFromEnv()

	assert.Empty(t, driver)
	assert.ErrorContains(t, err, "invalid BROWSER_RUNTIME_DRIVER")
}

func TestNewManagerFromEnvRejectsUnimplementedKubernetesDriver(t *testing.T) {
	t.Setenv(runtimeDriverEnv, browserruntime.DriverKubernetes)

	manager, err := NewManagerFromEnv()

	assert.Nil(t, manager)
	assert.ErrorContains(t, err, `browser runtime driver "kubernetes" is not implemented`)
}
