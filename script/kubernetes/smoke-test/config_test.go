package main

import (
	"strings"
	"testing"
)

func TestFullE2EEnablesRequiredProbeSet(t *testing.T) {
	config, err := ParseConfig(
		[]string{
			"--full-e2e",
			"--public-url",
			"https://mpp.example.com",
			"--auth-token",
			"token",
			"--project-id",
			"project-1",
		},
		map[string]string{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !config.RunUserFlowProbes {
		t.Fatal("expected full E2E to enable user flow probes")
	}
	if !config.RunBrowserSessionProbe {
		t.Fatal("expected full E2E to enable browser session probe")
	}
	if !config.RequireUserFlows {
		t.Fatal("expected full E2E to require user-flow inputs")
	}
}

func TestFullE2ERequiresProjectID(t *testing.T) {
	_, err := ParseConfig(
		[]string{
			"--full-e2e",
			"--public-url",
			"https://mpp.example.com",
			"--auth-token",
			"token",
		},
		map[string]string{},
	)

	if err == nil || !strings.Contains(err.Error(), "project-id") {
		t.Fatalf("expected missing project-id error, got %v", err)
	}
}

func TestFullE2ERejectsCoreSkipFlags(t *testing.T) {
	_, err := ParseConfig(
		[]string{
			"--full-e2e",
			"--skip-runtime-cleanup",
			"--public-url",
			"https://mpp.example.com",
			"--auth-token",
			"token",
			"--project-id",
			"project-1",
		},
		map[string]string{},
	)

	if err == nil || !strings.Contains(err.Error(), "skip flags") {
		t.Fatalf("expected skip flag conflict error, got %v", err)
	}
}
