package runtimefactory

import (
	"fmt"
	"os"
	"strings"

	browsercontainer "github.com/kurodakayn/mpp-browser-worker/internal/container"
	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

const runtimeDriverEnv = "BROWSER_RUNTIME_DRIVER"

func NewManagerFromEnv() (browserruntime.Manager, error) {
	driver, err := DriverFromEnv()
	if err != nil {
		return nil, err
	}

	switch driver {
	case browserruntime.DriverDocker:
		return browsercontainer.NewManager()
	case browserruntime.DriverKubernetes:
		return nil, fmt.Errorf("browser runtime driver %q is not implemented", driver)
	default:
		return nil, fmt.Errorf("unsupported browser runtime driver %q", driver)
	}
}

func DriverFromEnv() (string, error) {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv(runtimeDriverEnv)))
	if driver == "" {
		return browserruntime.DriverDocker, nil
	}
	switch driver {
	case browserruntime.DriverDocker, browserruntime.DriverKubernetes:
		return driver, nil
	default:
		return "", fmt.Errorf("invalid %s %q: expected docker or kubernetes", runtimeDriverEnv, driver)
	}
}
