package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kurodakayn/mpp-backend/internal/services/email"
)

const (
	JWTSecretEnv               = "JWT_SECRET"
	CollabTokenSecretEnv       = "COLLAB_TOKEN_SECRET" //nolint:gosec // This is an environment variable name, not a secret value.
	CollabInternalURLEnv       = "COLLAB_INTERNAL_URL"
	CollabWebsocketURLBaseEnv  = "COLLAB_WEBSOCKET_URL_BASE"
	AppEnvEnv                  = "APP_ENV"
	MockLoginFlagEnv           = "ENABLE_MOCK_LOGIN"
	NodeEnvFallbackEnv         = "NODE_ENV"
	BackendProcessRoleEnv      = "BACKEND_PROCESS_ROLE"
	BackendRequireRedisEnv     = "BACKEND_REQUIRE_REDIS"
	ExtensionAllowedOriginsEnv = "EXTENSION_ALLOWED_ORIGINS"

	ProcessRoleAll    = "all"
	ProcessRoleAPI    = "api"
	ProcessRoleWorker = "worker"

	BackendServiceName       = "backend"
	PublishWorkerServiceName = "publish-worker"
)

const (
	backendDefaultProcessRole  = ProcessRoleAll
	backendDefaultRequireRedis = false
	defaultCollabWebsocketURL  = "ws://localhost:8090"
	defaultCollabInternalURL   = "http://localhost:8090"
)

type RuntimeConfig struct {
	ProcessRole             string
	RequireRedis            bool
	ExtensionAllowedOrigins []string
}

func RuntimeConfigFromEnv() (RuntimeConfig, error) {
	processRole, err := processRoleFromEnv()
	if err != nil {
		return RuntimeConfig{}, err
	}
	extensionAllowedOrigins, err := CommaSeparatedEnv(ExtensionAllowedOriginsEnv)
	if err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{
		ProcessRole:             processRole,
		RequireRedis:            EnvFlagWithDefault(BackendRequireRedisEnv, backendDefaultRequireRedis),
		ExtensionAllowedOrigins: extensionAllowedOrigins,
	}, nil
}

func processRoleFromEnv() (string, error) {
	processRole := strings.ToLower(strings.TrimSpace(os.Getenv(BackendProcessRoleEnv)))
	if processRole == "" {
		processRole = backendDefaultProcessRole
	}
	switch processRole {
	case ProcessRoleAll, ProcessRoleAPI, ProcessRoleWorker:
		return processRole, nil
	default:
		return "", fmt.Errorf("%s must be one of: %s, %s, %s", BackendProcessRoleEnv, ProcessRoleAll, ProcessRoleAPI, ProcessRoleWorker)
	}
}

func (c RuntimeConfig) ServesAPI() bool {
	return c.ProcessRole == ProcessRoleAll || c.ProcessRole == ProcessRoleAPI
}

func (c RuntimeConfig) RunsWorkers() bool {
	return c.ProcessRole == ProcessRoleAll || c.ProcessRole == ProcessRoleWorker
}

func (c RuntimeConfig) ServiceName() string {
	if c.ProcessRole == ProcessRoleWorker {
		return PublishWorkerServiceName
	}
	return BackendServiceName
}

func RequiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return value, nil
}

func CollabTokenSecret(jwtSecret string) string {
	if value := strings.TrimSpace(os.Getenv(CollabTokenSecretEnv)); value != "" {
		return value
	}
	return jwtSecret
}

func CollabWebsocketURLBase() string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv(CollabWebsocketURLBaseEnv)), "/"); value != "" {
		return value
	}
	return defaultCollabWebsocketURL
}

func CollabInternalURL() string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv(CollabInternalURLEnv)), "/"); value != "" {
		return value
	}
	return defaultCollabInternalURL
}

func CommaSeparatedEnv(name string) ([]string, error) {
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

func MockLoginEnabled() bool {
	localEnv := isLocalEnvironment(os.Getenv(AppEnvEnv)) || isLocalEnvironment(os.Getenv(NodeEnvFallbackEnv))
	return EnvFlagEnabled(MockLoginFlagEnv) && localEnv
}

func EnvFlagEnabled(name string) bool {
	return EnvFlagWithDefault(name, false)
}

func EnvFlagWithDefault(name string, defaultValue bool) bool {
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

func NewBaseEmailServiceFromEnv() (email.EmailService, error) {
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		return &email.MockEmailService{}, nil
	}

	smtpPort := 587
	if rawPort := strings.TrimSpace(os.Getenv("SMTP_PORT")); rawPort != "" {
		parsedPort, err := strconv.Atoi(rawPort)
		if err != nil || parsedPort <= 0 {
			return nil, fmt.Errorf("invalid SMTP_PORT: must be a positive integer")
		}
		smtpPort = parsedPort
	}
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	smtpPassword := strings.TrimSpace(os.Getenv("SMTP_PASSWORD"))
	if smtpFrom == "" || smtpPassword == "" {
		return nil, fmt.Errorf("SMTP_FROM and SMTP_PASSWORD must be set when SMTP_HOST is set")
	}

	return email.NewSMTPEmailService(smtpHost, smtpPort, smtpFrom, smtpPassword), nil
}
