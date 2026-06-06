package app

import (
	"strings"
	"testing"
)

func TestProcessRoleFromEnvDefaultsToAll(t *testing.T) {
	t.Setenv(BackendProcessRoleEnv, "")

	role, err := processRoleFromEnv()
	if err != nil {
		t.Fatalf("expected default role to be accepted: %v", err)
	}
	if role != ProcessRoleAll {
		t.Fatalf("expected default role %q, got %q", ProcessRoleAll, role)
	}
}

func TestProcessRoleFromEnvAcceptsKnownRoles(t *testing.T) {
	for _, role := range []string{ProcessRoleAll, ProcessRoleAPI, ProcessRoleWorker, " API "} {
		t.Run(role, func(t *testing.T) {
			t.Setenv(BackendProcessRoleEnv, role)

			got, err := processRoleFromEnv()
			if err != nil {
				t.Fatalf("expected role to be accepted: %v", err)
			}
			expected := strings.ToLower(strings.TrimSpace(role))
			if got != expected {
				t.Fatalf("expected normalized role %q, got %q", expected, got)
			}
		})
	}
}

func TestProcessRoleFromEnvRejectsUnknownRole(t *testing.T) {
	t.Setenv(BackendProcessRoleEnv, "sidecar")

	if _, err := processRoleFromEnv(); err == nil {
		t.Fatal("expected unknown backend process role to be rejected")
	}
}

func TestRuntimeConfigRoleCapabilities(t *testing.T) {
	api := RuntimeConfig{ProcessRole: ProcessRoleAPI}
	if !api.ServesAPI() || api.RunsWorkers() {
		t.Fatal("api role must serve API without running workers")
	}

	worker := RuntimeConfig{ProcessRole: ProcessRoleWorker}
	if worker.ServesAPI() || !worker.RunsWorkers() {
		t.Fatal("worker role must run workers without serving API")
	}

	all := RuntimeConfig{ProcessRole: ProcessRoleAll}
	if !all.ServesAPI() || !all.RunsWorkers() {
		t.Fatal("all role must serve API and run workers")
	}
}

func TestRuntimeConfigReadsRequireRedisFlag(t *testing.T) {
	t.Setenv(BackendProcessRoleEnv, ProcessRoleAPI)
	t.Setenv(BackendRequireRedisEnv, "true")

	config, err := RuntimeConfigFromEnv()
	if err != nil {
		t.Fatalf("expected runtime config: %v", err)
	}
	if !config.RequireRedis {
		t.Fatal("expected redis to be required when flag is true")
	}
}

func TestRuntimeConfigReadsExtensionAllowedOrigins(t *testing.T) {
	t.Setenv(BackendProcessRoleEnv, ProcessRoleAPI)
	t.Setenv(ExtensionAllowedOriginsEnv, " chrome-extension://abc , http://localhost:3000 ,, ")

	config, err := RuntimeConfigFromEnv()
	if err != nil {
		t.Fatalf("expected runtime config: %v", err)
	}

	expectedOrigins := []string{"chrome-extension://abc", "http://localhost:3000"}
	if len(config.ExtensionAllowedOrigins) != len(expectedOrigins) {
		t.Fatalf("expected %d extension origins, got %d", len(expectedOrigins), len(config.ExtensionAllowedOrigins))
	}
	for index, expected := range expectedOrigins {
		if config.ExtensionAllowedOrigins[index] != expected {
			t.Fatalf("expected extension origin %q at index %d, got %q", expected, index, config.ExtensionAllowedOrigins[index])
		}
	}
}

func TestRuntimeConfigRejectsWildcardExtensionAllowedOrigin(t *testing.T) {
	t.Setenv(BackendProcessRoleEnv, ProcessRoleAPI)
	t.Setenv(ExtensionAllowedOriginsEnv, "chrome-extension://abc,*")

	if _, err := RuntimeConfigFromEnv(); err == nil {
		t.Fatal("expected wildcard extension origin to be rejected")
	}
}

func TestRequiredEnvRejectsMissingAndBlankValues(t *testing.T) {
	t.Setenv("TEST_REQUIRED_ENV", "")

	if _, err := RequiredEnv("TEST_REQUIRED_ENV"); err == nil {
		t.Fatal("expected blank env value to be rejected")
	}
}

func TestRequiredEnvReturnsTrimmedValue(t *testing.T) {
	t.Setenv("TEST_REQUIRED_ENV", "  secret-value  ")

	value, err := RequiredEnv("TEST_REQUIRED_ENV")
	if err != nil {
		t.Fatalf("expected env value to be accepted: %v", err)
	}
	if value != "secret-value" {
		t.Fatalf("expected trimmed env value, got %q", value)
	}
}

func TestMockLoginEnabledRequiresExplicitFlagAndLocalEnvironment(t *testing.T) {
	t.Setenv(MockLoginFlagEnv, "true")
	t.Setenv(AppEnvEnv, "production")
	t.Setenv(NodeEnvFallbackEnv, "")

	if MockLoginEnabled() {
		t.Fatal("mock login must not be enabled outside local development")
	}

	t.Setenv(AppEnvEnv, "development")

	if !MockLoginEnabled() {
		t.Fatal("mock login should be enabled when explicitly flagged in local development")
	}
}

func TestMockLoginEnabledFallsBackToNodeEnvForLocalDevelopment(t *testing.T) {
	t.Setenv(MockLoginFlagEnv, "true")
	t.Setenv(AppEnvEnv, "")
	t.Setenv(NodeEnvFallbackEnv, "development")

	if !MockLoginEnabled() {
		t.Fatal("mock login should be enabled when NODE_ENV marks local development")
	}
}

func TestSecureCookiesByDefaultOutsideLocalDevelopment(t *testing.T) {
	t.Setenv(AppEnvEnv, "production")
	t.Setenv(NodeEnvFallbackEnv, "")

	if !SecureCookiesByDefault() {
		t.Fatal("secure cookies should be enabled outside local development")
	}
}

func TestSecureCookiesByDefaultPrefersAppEnvOverNodeEnv(t *testing.T) {
	t.Setenv(AppEnvEnv, "production")
	t.Setenv(NodeEnvFallbackEnv, "development")

	if !SecureCookiesByDefault() {
		t.Fatal("secure cookies should stay enabled when APP_ENV is production")
	}
}

func TestSecureCookiesByDefaultDisabledForLocalDevelopment(t *testing.T) {
	t.Setenv(AppEnvEnv, "development")
	t.Setenv(NodeEnvFallbackEnv, "")

	if SecureCookiesByDefault() {
		t.Fatal("secure cookies should be disabled for local development")
	}
}

func TestSecureCookiesByDefaultFallsBackToNodeEnvForLocalDevelopment(t *testing.T) {
	t.Setenv(AppEnvEnv, "")
	t.Setenv(NodeEnvFallbackEnv, "development")

	if SecureCookiesByDefault() {
		t.Fatal("secure cookies should be disabled when only NODE_ENV marks local development")
	}
}

func TestNewBaseEmailServiceFromEnvRejectsInvalidSMTPPort(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "invalid")
	t.Setenv("SMTP_FROM", "noreply@example.test")
	t.Setenv("SMTP_PASSWORD", "password")

	if _, err := NewBaseEmailServiceFromEnv(); err == nil {
		t.Fatal("expected invalid SMTP_PORT to be rejected")
	}
}
