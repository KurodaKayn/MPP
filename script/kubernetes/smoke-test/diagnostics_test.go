package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestCollectsFailureDiagnosticsWhenChecksFail(t *testing.T) {
	var stdout bytes.Buffer
	reporter := NewReporter(&stdout, false)
	reporter.Fail("forced failure", "boom")
	runner := &fakeDiagnosticRunner{
		outputs: map[string]string{
			"get pods -n mpp-system -l app.kubernetes.io/name=mpp -o wide": "app pods\n",
			"get events -n mpp-system --sort-by=.lastTimestamp":            "app events\n",
			"describe deployment/browser-worker -n mpp-system":             "browser worker\n",
			"get pods -n mpp-browser-runtime -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime,mpp.kurodakayn.dev/runtime-driver=kubernetes -o wide": "runtime pods\n",
			"get events -n mpp-browser-runtime --sort-by=.lastTimestamp": "runtime events\n",
		},
	}

	maybeCollectDiagnostics(testConfig(), reporter, runner)

	output := stdout.String()
	for _, expected := range []string{
		"Failure Diagnostics:",
		"DIAG app Pods",
		"app pods",
		"DIAG app events",
		"DIAG browser-worker describe",
		"DIAG runtime Pods",
		"DIAG runtime events",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected diagnostics output to contain %q, got:\n%s", expected, output)
		}
	}
	if len(runner.commands) != 5 {
		t.Fatalf("expected five diagnostic commands, got %#v", runner.commands)
	}
}

func TestSkipsFailureDiagnosticsWhenDisabled(t *testing.T) {
	var stdout bytes.Buffer
	reporter := NewReporter(&stdout, false)
	reporter.Fail("forced failure", "boom")
	config := testConfig()
	config.SkipDiagnostics = true
	runner := &fakeDiagnosticRunner{}

	maybeCollectDiagnostics(config, reporter, runner)

	if len(runner.commands) != 0 {
		t.Fatalf("expected no diagnostic commands, got %#v", runner.commands)
	}
	if strings.Contains(stdout.String(), "Failure Diagnostics") {
		t.Fatalf("did not expect diagnostics section, got:\n%s", stdout.String())
	}
}

func TestDiagnosticsReportCommandFailuresAsWarnings(t *testing.T) {
	var stdout bytes.Buffer
	reporter := NewReporter(&stdout, false)
	reporter.Fail("forced failure", "boom")
	runner := &fakeDiagnosticRunner{err: errors.New("diagnostic failed")}

	maybeCollectDiagnostics(testConfig(), reporter, runner)

	if len(reporter.Warnings) != 5 {
		t.Fatalf("expected five diagnostic warnings, got %#v", reporter.Warnings)
	}
	if !strings.Contains(stdout.String(), "WARN app Pods") {
		t.Fatalf("expected diagnostic warning output, got:\n%s", stdout.String())
	}
}

func TestTailLinesKeepsLastLines(t *testing.T) {
	result := tailLines("one\ntwo\nthree\n", 2)

	if result != "two\nthree" {
		t.Fatalf("expected last two lines, got %q", result)
	}
}

func TestDiagnosticLineLimitCanBeConfigured(t *testing.T) {
	config, err := ParseConfig([]string{"--diagnostic-lines", "12"}, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}

	if config.DiagnosticLines != 12 {
		t.Fatalf("expected diagnostic line limit 12, got %d", config.DiagnosticLines)
	}
}

func testConfig() *Config {
	return &Config{
		AppNamespace:     "mpp-system",
		RuntimeNamespace: "mpp-browser-runtime",
		DiagnosticLines:  60,
	}
}

type fakeDiagnosticRunner struct {
	outputs  map[string]string
	err      error
	commands []string
}

func (runner *fakeDiagnosticRunner) Run(args []string, options RunOptions) (string, error) {
	command := strings.Join(args, " ")
	runner.commands = append(runner.commands, command)
	if runner.err != nil {
		return "", runner.err
	}
	return runner.outputs[command], nil
}
