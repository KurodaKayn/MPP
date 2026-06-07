package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type CheckFailure string

func (err CheckFailure) Error() string {
	return string(err)
}

type CheckSkip string

func (err CheckSkip) Error() string {
	return string(err)
}

type CheckResult struct {
	Name   string
	Detail string
}

type Reporter struct {
	io       io.Writer
	verbose  bool
	section  string
	Passes   []CheckResult
	Failures []CheckResult
	Warnings []CheckResult
	Skips    []CheckResult
}

func NewReporter(io io.Writer, verbose bool) *Reporter {
	return &Reporter{io: io, verbose: verbose}
}

func (reporter *Reporter) Section(title string) {
	if reporter.section == title {
		return
	}
	reporter.section = title
	fmt.Fprintln(reporter.io)
	fmt.Fprintf(reporter.io, "%s:\n", title)
}

func (reporter *Reporter) Check(name string, required bool, fn func() (string, error)) {
	detail, err := fn()
	if err == nil {
		reporter.Pass(name, detail)
		return
	}

	var checkSkip CheckSkip
	if errors.As(err, &checkSkip) {
		reporter.Skip(name, checkSkip.Error())
		return
	}

	var commandError *CommandError
	if errors.As(err, &commandError) {
		reporter.recordError(name, commandError.Message(), required)
		return
	}

	var checkFailure CheckFailure
	if errors.As(err, &checkFailure) {
		reporter.recordError(name, checkFailure.Error(), required)
		return
	}

	reporter.recordError(name, fmt.Sprintf("%T: %v", err, err), required)
}

func (reporter *Reporter) Pass(name string, detail string) {
	reporter.Passes = append(reporter.Passes, CheckResult{Name: name, Detail: detail})
	reporter.line("PASS", name, detail)
}

func (reporter *Reporter) Fail(name string, detail string) {
	reporter.Failures = append(reporter.Failures, CheckResult{Name: name, Detail: detail})
	reporter.line("FAIL", name, detail)
}

func (reporter *Reporter) Warn(name string, detail string) {
	reporter.Warnings = append(reporter.Warnings, CheckResult{Name: name, Detail: detail})
	reporter.line("WARN", name, detail)
}

func (reporter *Reporter) Skip(name string, detail string) {
	reporter.Skips = append(reporter.Skips, CheckResult{Name: name, Detail: detail})
	reporter.line("SKIP", name, detail)
}

func (reporter *Reporter) Info(message string) {
	if reporter.verbose {
		fmt.Fprintf(reporter.io, "  INFO %s\n", message)
	}
}

func (reporter *Reporter) Command(command []string, dryRun bool) {
	if dryRun {
		fmt.Fprintf(reporter.io, "  DRY-RUN %s\n", strings.Join(command, " "))
		return
	}
	reporter.Info("$ " + strings.Join(command, " "))
}

func (reporter *Reporter) Summary() {
	fmt.Fprintln(reporter.io)
	fmt.Fprintln(reporter.io, "Summary:")
	fmt.Fprintf(reporter.io, "  passes: %d\n", len(reporter.Passes))
	fmt.Fprintf(reporter.io, "  warnings: %d\n", len(reporter.Warnings))
	fmt.Fprintf(reporter.io, "  skips: %d\n", len(reporter.Skips))
	fmt.Fprintf(reporter.io, "  failures: %d\n", len(reporter.Failures))
	if len(reporter.Failures) == 0 {
		return
	}

	fmt.Fprintln(reporter.io)
	fmt.Fprintln(reporter.io, "Failures:")
	for _, failure := range reporter.Failures {
		fmt.Fprintf(reporter.io, "  - %s: %s\n", failure.Name, failure.Detail)
	}
}

func (reporter *Reporter) ExitCode() int {
	reporter.Summary()
	if len(reporter.Failures) == 0 {
		return 0
	}
	return 1
}

func (reporter *Reporter) line(status string, name string, detail string) {
	if detail == "" {
		fmt.Fprintf(reporter.io, "  %s %s\n", status, name)
		return
	}
	fmt.Fprintf(reporter.io, "  %s %s - %s\n", status, name, detail)
}

func (reporter *Reporter) recordError(name string, detail string, required bool) {
	if required {
		reporter.Fail(name, detail)
		return
	}
	reporter.Warn(name, detail)
}
