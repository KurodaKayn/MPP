package main

import (
	"strings"
	"testing"
)

func TestBackendProcessRoleFromEnvDefaultsToAll(t *testing.T) {
	t.Setenv(backendProcessRoleEnv, "")

	role, err := backendProcessRoleFromEnv()
	if err != nil {
		t.Fatalf("expected default role to be accepted: %v", err)
	}
	if role != backendProcessRoleAll {
		t.Fatalf("expected default role %q, got %q", backendProcessRoleAll, role)
	}
}

func TestBackendProcessRoleFromEnvAcceptsKnownRoles(t *testing.T) {
	for _, role := range []string{backendProcessRoleAll, backendProcessRoleAPI, backendProcessRoleWorker, " API "} {
		t.Run(role, func(t *testing.T) {
			t.Setenv(backendProcessRoleEnv, role)

			got, err := backendProcessRoleFromEnv()
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

func TestBackendProcessRoleFromEnvRejectsUnknownRole(t *testing.T) {
	t.Setenv(backendProcessRoleEnv, "sidecar")

	if _, err := backendProcessRoleFromEnv(); err == nil {
		t.Fatal("expected unknown backend process role to be rejected")
	}
}

func TestBackendRuntimeConfigRoleCapabilities(t *testing.T) {
	api := backendRuntimeConfig{processRole: backendProcessRoleAPI}
	if !api.servesAPI() || api.runsWorkers() {
		t.Fatal("api role must serve API without running workers")
	}

	worker := backendRuntimeConfig{processRole: backendProcessRoleWorker}
	if worker.servesAPI() || !worker.runsWorkers() {
		t.Fatal("worker role must run workers without serving API")
	}

	all := backendRuntimeConfig{processRole: backendProcessRoleAll}
	if !all.servesAPI() || !all.runsWorkers() {
		t.Fatal("all role must serve API and run workers")
	}
}

func TestBackendRuntimeConfigReadsRequireRedisFlag(t *testing.T) {
	t.Setenv(backendProcessRoleEnv, backendProcessRoleAPI)
	t.Setenv(backendRequireRedisEnv, "true")

	config, err := backendRuntimeConfigFromEnv()
	if err != nil {
		t.Fatalf("expected runtime config: %v", err)
	}
	if !config.requireRedis {
		t.Fatal("expected redis to be required when flag is true")
	}
}

func TestBackendRuntimeConfigReadsExtensionAllowedOrigins(t *testing.T) {
	t.Setenv(backendProcessRoleEnv, backendProcessRoleAPI)
	t.Setenv(extensionAllowedOriginsEnv, " chrome-extension://abc , http://localhost:3000 ,, ")

	config, err := backendRuntimeConfigFromEnv()
	if err != nil {
		t.Fatalf("expected runtime config: %v", err)
	}

	expectedOrigins := []string{"chrome-extension://abc", "http://localhost:3000"}
	if len(config.extensionAllowedOrigins) != len(expectedOrigins) {
		t.Fatalf("expected %d extension origins, got %d", len(expectedOrigins), len(config.extensionAllowedOrigins))
	}
	for index, expected := range expectedOrigins {
		if config.extensionAllowedOrigins[index] != expected {
			t.Fatalf("expected extension origin %q at index %d, got %q", expected, index, config.extensionAllowedOrigins[index])
		}
	}
}

func TestBackendRuntimeConfigRejectsWildcardExtensionAllowedOrigin(t *testing.T) {
	t.Setenv(backendProcessRoleEnv, backendProcessRoleAPI)
	t.Setenv(extensionAllowedOriginsEnv, "chrome-extension://abc,*")

	if _, err := backendRuntimeConfigFromEnv(); err == nil {
		t.Fatal("expected wildcard extension origin to be rejected")
	}
}

func TestRequiredEnvRejectsMissingAndBlankValues(t *testing.T) {
	t.Setenv("TEST_REQUIRED_ENV", "")

	if _, err := requiredEnv("TEST_REQUIRED_ENV"); err == nil {
		t.Fatal("expected blank env value to be rejected")
	}
}

func TestRequiredEnvReturnsTrimmedValue(t *testing.T) {
	t.Setenv("TEST_REQUIRED_ENV", "  secret-value  ")

	value, err := requiredEnv("TEST_REQUIRED_ENV")
	if err != nil {
		t.Fatalf("expected env value to be accepted: %v", err)
	}
	if value != "secret-value" {
		t.Fatalf("expected trimmed env value, got %q", value)
	}
}

func TestMockLoginEnabledRequiresExplicitFlagAndLocalEnvironment(t *testing.T) {
	t.Setenv(mockLoginFlagEnv, "true")
	t.Setenv(appEnvEnv, "production")
	t.Setenv(nodeEnvFallbackEnv, "")

	if mockLoginEnabled() {
		t.Fatal("mock login must not be enabled outside local development")
	}

	t.Setenv(appEnvEnv, "development")

	if !mockLoginEnabled() {
		t.Fatal("mock login should be enabled when explicitly flagged in local development")
	}
}

func TestMockLoginEnabledFallsBackToNodeEnvForLocalDevelopment(t *testing.T) {
	t.Setenv(mockLoginFlagEnv, "true")
	t.Setenv(appEnvEnv, "")
	t.Setenv(nodeEnvFallbackEnv, "development")

	if !mockLoginEnabled() {
		t.Fatal("mock login should be enabled when NODE_ENV marks local development")
	}
}
