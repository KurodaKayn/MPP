package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDryRunExitsSuccessfullyAndPrintsKubectlIntent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	status := run([]string{"--dry-run", "--skip-public"}, map[string]string{}, &stdout, &stderr)

	if status != 0 {
		t.Fatalf("expected success, got status %d\nstdout:\n%s\nstderr:\n%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "DRY-RUN kubectl config current-context") {
		t.Fatalf("expected dry-run kubectl intent, got:\n%s", stdout.String())
	}
}
