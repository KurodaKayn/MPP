package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	jwtSecretEnv               = "JWT_SECRET"
	appEnvEnv                  = "APP_ENV"
	mockLoginFlagEnv           = "ENABLE_MOCK_LOGIN"
	nodeEnvFallbackEnv         = "NODE_ENV"
	backendProcessRoleEnv      = "BACKEND_PROCESS_ROLE"
	backendRequireRedisEnv     = "BACKEND_REQUIRE_REDIS"
	backendProcessRoleAll      = "all"
	backendProcessRoleAPI      = "api"
	backendProcessRoleWorker   = "worker"
	backendServiceName         = "backend"
	backendWorkerServiceName   = "publish-worker"
	backendDefaultProcessRole  = backendProcessRoleAll
	backendDefaultRequireRedis = false
)

type backendRuntimeConfig struct {
	processRole  string
	requireRedis bool
}

func backendRuntimeConfigFromEnv() (backendRuntimeConfig, error) {
	processRole, err := backendProcessRoleFromEnv()
	if err != nil {
		return backendRuntimeConfig{}, err
	}
	return backendRuntimeConfig{
		processRole:  processRole,
		requireRedis: envFlagWithDefault(backendRequireRedisEnv, backendDefaultRequireRedis),
	}, nil
}

func backendProcessRoleFromEnv() (string, error) {
	processRole := strings.ToLower(strings.TrimSpace(os.Getenv(backendProcessRoleEnv)))
	if processRole == "" {
		processRole = backendDefaultProcessRole
	}
	switch processRole {
	case backendProcessRoleAll, backendProcessRoleAPI, backendProcessRoleWorker:
		return processRole, nil
	default:
		return "", fmt.Errorf("%s must be one of: %s, %s, %s", backendProcessRoleEnv, backendProcessRoleAll, backendProcessRoleAPI, backendProcessRoleWorker)
	}
}

func (c backendRuntimeConfig) servesAPI() bool {
	return c.processRole == backendProcessRoleAll || c.processRole == backendProcessRoleAPI
}

func (c backendRuntimeConfig) runsWorkers() bool {
	return c.processRole == backendProcessRoleAll || c.processRole == backendProcessRoleWorker
}

func (c backendRuntimeConfig) serviceName() string {
	if c.processRole == backendProcessRoleWorker {
		return backendWorkerServiceName
	}
	return backendServiceName
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return value, nil
}

func mockLoginEnabled() bool {
	localEnv := isLocalEnvironment(os.Getenv(appEnvEnv)) || isLocalEnvironment(os.Getenv(nodeEnvFallbackEnv))
	return envFlagEnabled(mockLoginFlagEnv) && localEnv
}

func envFlagEnabled(name string) bool {
	return envFlagWithDefault(name, false)
}

func envFlagWithDefault(name string, defaultValue bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func isLocalEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "dev", "development":
		return true
	default:
		return false
	}
}
