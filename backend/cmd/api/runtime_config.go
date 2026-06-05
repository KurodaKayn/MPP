package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	jwtSecretEnv               = "JWT_SECRET"
	collabTokenSecretEnv       = "COLLAB_TOKEN_SECRET" //nolint:gosec // This is an environment variable name, not a secret value.
	collabInternalURLEnv       = "COLLAB_INTERNAL_URL"
	collabWebsocketURLBaseEnv  = "COLLAB_WEBSOCKET_URL_BASE"
	appEnvEnv                  = "APP_ENV"
	mockLoginFlagEnv           = "ENABLE_MOCK_LOGIN"
	nodeEnvFallbackEnv         = "NODE_ENV"
	backendProcessRoleEnv      = "BACKEND_PROCESS_ROLE"
	backendRequireRedisEnv     = "BACKEND_REQUIRE_REDIS"
	extensionAllowedOriginsEnv = "EXTENSION_ALLOWED_ORIGINS"
	backendProcessRoleAll      = "all"
	backendProcessRoleAPI      = "api"
	backendProcessRoleWorker   = "worker"
	backendServiceName         = "backend"
	backendWorkerServiceName   = "publish-worker"
	backendDefaultProcessRole  = backendProcessRoleAll
	backendDefaultRequireRedis = false
	defaultCollabWebsocketURL  = "ws://localhost:8090"
	defaultCollabInternalURL   = "http://localhost:8090"
)

type backendRuntimeConfig struct {
	processRole             string
	requireRedis            bool
	extensionAllowedOrigins []string
}

func backendRuntimeConfigFromEnv() (backendRuntimeConfig, error) {
	processRole, err := backendProcessRoleFromEnv()
	if err != nil {
		return backendRuntimeConfig{}, err
	}
	extensionAllowedOrigins, err := commaSeparatedEnv(extensionAllowedOriginsEnv)
	if err != nil {
		return backendRuntimeConfig{}, err
	}
	return backendRuntimeConfig{
		processRole:             processRole,
		requireRedis:            envFlagWithDefault(backendRequireRedisEnv, backendDefaultRequireRedis),
		extensionAllowedOrigins: extensionAllowedOrigins,
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

func collabTokenSecret(jwtSecret string) string {
	if value := strings.TrimSpace(os.Getenv(collabTokenSecretEnv)); value != "" {
		return value
	}
	return jwtSecret
}

func collabWebsocketURLBase() string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv(collabWebsocketURLBaseEnv)), "/"); value != "" {
		return value
	}
	return defaultCollabWebsocketURL
}

func collabInternalURL() string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv(collabInternalURLEnv)), "/"); value != "" {
		return value
	}
	return defaultCollabInternalURL
}

func commaSeparatedEnv(name string) ([]string, error) {
	rawValues := strings.Split(os.Getenv(name), ",")
	values := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}
		if strings.Contains(value, "*") {
			return nil, fmt.Errorf("%s must not contain wildcard origins when credentials are enabled", name)
		}
		values = append(values, value)
	}
	return values, nil
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
