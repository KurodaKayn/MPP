package runtimefactory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
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

func TestNewManagerFromEnvLoadsKubernetesDriverConfig(t *testing.T) {
	t.Setenv(runtimeDriverEnv, browserruntime.DriverKubernetes)
	t.Setenv("BROWSER_RUNTIME_KUBERNETES_KUBECONFIG", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	manager, err := NewManagerFromEnv()

	assert.Nil(t, manager)
	assert.ErrorContains(t, err, "failed to load kubernetes config")
}
