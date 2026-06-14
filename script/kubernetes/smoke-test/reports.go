package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type smokeReport struct {
	GeneratedAt string        `json:"generated_at"`
	Summary     reportSummary `json:"summary"`
	Passes      []CheckResult `json:"passes"`
	Failures    []CheckResult `json:"failures"`
	Warnings    []CheckResult `json:"warnings"`
	Skips       []CheckResult `json:"skips"`
}

type reportSummary struct {
	Passes   int `json:"passes"`
	Failures int `json:"failures"`
	Warnings int `json:"warnings"`
	Skips    int `json:"skips"`
	Total    int `json:"total"`
}

type junitTestsuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Skipped  int              `xml:"skipped,attr"`
	Suites   []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Testcases []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

func writeReports(config *Config, reporter *Reporter) []error {
	errors := make([]error, 0)
	if config.ReportJSON != "" {
		if err := writeJSONReport(config.ReportJSON, reporter); err != nil {
			errors = append(errors, err)
		}
	}
	if config.ReportJUnit != "" {
		if err := writeJUnitReport(config.ReportJUnit, reporter); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func writeJSONReport(path string, reporter *Reporter) error {
	payload, err := json.MarshalIndent(newSmokeReport(reporter), "", "  ")
	if err != nil {
		return fmt.Errorf("build JSON report %s: %w", path, err)
	}
	if err := writeFile(path, append(payload, '\n')); err != nil {
		return fmt.Errorf("write JSON report %s: %w", path, err)
	}
	return nil
}

func writeJUnitReport(path string, reporter *Reporter) error {
	payload, err := xml.MarshalIndent(newJUnitReport(reporter), "", "  ")
	if err != nil {
		return fmt.Errorf("build JUnit report %s: %w", path, err)
	}
	payload = append([]byte(xml.Header), payload...)
	payload = append(payload, '\n')
	if err := writeFile(path, payload); err != nil {
		return fmt.Errorf("write JUnit report %s: %w", path, err)
	}
	return nil
}

func newSmokeReport(reporter *Reporter) smokeReport {
	summary := reporterSummary(reporter)
	return smokeReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     summary,
		Passes:      cloneResults(reporter.Passes),
		Failures:    cloneResults(reporter.Failures),
		Warnings:    cloneResults(reporter.Warnings),
		Skips:       cloneResults(reporter.Skips),
	}
}

func newJUnitReport(reporter *Reporter) junitTestsuites {
	summary := reporterSummary(reporter)
	testcases := make([]junitTestcase, 0, summary.Total)
	for _, result := range reporter.Passes {
		testcases = append(testcases, junitTestcase{
			Classname: "mpp.kubernetes_smoke",
			Name:      result.Name,
			SystemOut: result.Detail,
		})
	}
	for _, result := range reporter.Failures {
		testcases = append(testcases, junitTestcase{
			Classname: "mpp.kubernetes_smoke",
			Name:      result.Name,
			Failure:   &junitFailure{Message: result.Detail, Body: result.Detail},
		})
	}
	for _, result := range reporter.Warnings {
		testcases = append(testcases, junitTestcase{
			Classname: "mpp.kubernetes_smoke",
			Name:      result.Name,
			SystemOut: "WARNING: " + result.Detail,
		})
	}
	for _, result := range reporter.Skips {
		testcases = append(testcases, junitTestcase{
			Classname: "mpp.kubernetes_smoke",
			Name:      result.Name,
			Skipped:   &junitSkipped{Message: result.Detail},
		})
	}
	return junitTestsuites{
		Tests:    summary.Total,
		Failures: summary.Failures,
		Skipped:  summary.Skips,
		Suites: []junitTestsuite{
			{
				Name:      "mpp-kubernetes-smoke",
				Tests:     summary.Total,
				Failures:  summary.Failures,
				Skipped:   summary.Skips,
				Testcases: testcases,
			},
		},
	}
}

func reporterSummary(reporter *Reporter) reportSummary {
	summary := reportSummary{
		Passes:   len(reporter.Passes),
		Failures: len(reporter.Failures),
		Warnings: len(reporter.Warnings),
		Skips:    len(reporter.Skips),
	}
	summary.Total = summary.Passes + summary.Failures + summary.Warnings + summary.Skips
	return summary
}

func cloneResults(results []CheckResult) []CheckResult {
	return append([]CheckResult(nil), results...)
}

func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
