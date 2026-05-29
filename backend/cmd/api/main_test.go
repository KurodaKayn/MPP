package main

import "testing"

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
