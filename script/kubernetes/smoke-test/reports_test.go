package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesJSONAndJUnitReports(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "smoke.json")
	junitPath := filepath.Join(dir, "smoke.xml")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	status := run(
		[]string{
			"--dry-run",
			"--skip-public",
			"--report-json",
			jsonPath,
			"--report-junit",
			junitPath,
		},
		map[string]string{},
		&stdout,
		&stderr,
	)

	if status != 0 {
		t.Fatalf("expected success, got status %d\nstdout:\n%s\nstderr:\n%s", status, stdout.String(), stderr.String())
	}

	reportBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var report smokeReport
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Failures != 0 {
		t.Fatalf("expected no report failures, got %#v", report.Summary)
	}
	if report.Summary.Total == 0 {
		t.Fatalf("expected report test count, got %#v", report.Summary)
	}

	junitBytes, err := os.ReadFile(junitPath)
	if err != nil {
		t.Fatal(err)
	}
	var suites junitTestsuites
	if err := xml.Unmarshal(junitBytes, &suites); err != nil {
		t.Fatal(err)
	}
	if suites.Tests != report.Summary.Total {
		t.Fatalf("expected JUnit tests %d, got %d", report.Summary.Total, suites.Tests)
	}
	if suites.Failures != 0 {
		t.Fatalf("expected no JUnit failures, got %d", suites.Failures)
	}
}

func TestJSONReportWriteFailureMarksRunFailed(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dir := t.TempDir()

	status := run(
		[]string{
			"--dry-run",
			"--skip-public",
			"--report-json",
			dir,
		},
		map[string]string{},
		&stdout,
		&stderr,
	)

	if status == 0 {
		t.Fatalf("expected report write failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "write smoke report") {
		t.Fatalf("expected report write failure in stdout, got:\n%s", stdout.String())
	}
}
