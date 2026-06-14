package main

import (
	"errors"
	"fmt"
	"strings"
)

type diagnosticRunner interface {
	Run(args []string, options RunOptions) (string, error)
}

type diagnosticCommand struct {
	name string
	args []string
}

func maybeCollectDiagnostics(config *Config, reporter *Reporter, kubectl diagnosticRunner) {
	if config.SkipDiagnostics || len(reporter.Failures) == 0 {
		return
	}

	reporter.Section("Failure Diagnostics")
	for _, command := range diagnosticCommands(config) {
		output, err := kubectl.Run(command.args, RunOptions{})
		if err != nil {
			reporter.Warn(command.name, diagnosticErrorDetail(err))
			continue
		}
		reporter.Diagnostic(command.name, tailLines(output, config.DiagnosticLines))
	}
}

func diagnosticCommands(config *Config) []diagnosticCommand {
	appSelector := "app.kubernetes.io/name=mpp"
	runtimeSelector := "app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime,mpp.kurodakayn.dev/runtime-driver=kubernetes"
	return []diagnosticCommand{
		{
			name: "app Pods",
			args: []string{"get", "pods", "-n", config.AppNamespace, "-l", appSelector, "-o", "wide"},
		},
		{
			name: "app events",
			args: []string{"get", "events", "-n", config.AppNamespace, "--sort-by=.lastTimestamp"},
		},
		{
			name: "browser-worker describe",
			args: []string{"describe", "deployment/browser-worker", "-n", config.AppNamespace},
		},
		{
			name: "runtime Pods",
			args: []string{"get", "pods", "-n", config.RuntimeNamespace, "-l", runtimeSelector, "-o", "wide"},
		},
		{
			name: "runtime events",
			args: []string{"get", "events", "-n", config.RuntimeNamespace, "--sort-by=.lastTimestamp"},
		},
	}
}

func diagnosticErrorDetail(err error) string {
	var commandError *CommandError
	if errors.As(err, &commandError) {
		return commandError.Message()
	}
	return fmt.Sprintf("%T: %v", err, err)
}

func tailLines(text string, limit int) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return "(no output)"
	}
	if limit <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= limit {
		return text
	}
	return strings.Join(lines[len(lines)-limit:], "\n")
}
