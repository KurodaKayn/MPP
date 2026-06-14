package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/kurodakayn/mpp-kubernetes-smoke/checks"
)

var ErrHelp = errors.New("help requested")

type Config struct {
	AppNamespace           string
	RuntimeNamespace       string
	ObservabilityNamespace string
	RolloutTimeout         int
	RequestTimeout         int
	CurlImage              string
	PublicURL              string
	APIBaseURL             string
	AuthToken              string
	ProjectID              string
	BrowserPlatform        string
	FullE2E                bool
	RunUserFlowProbes      bool
	RunBrowserSessionProbe bool
	RequireUserFlows       bool
	SkipPublic             bool
	SkipInternalHTTP       bool
	SkipRuntimeRBAC        bool
	SkipRuntimeCleanup     bool
	SkipDiagnostics        bool
	DiagnosticLines        int
	ReportJSON             string
	ReportJUnit            string
	DryRun                 bool
	Verbose                bool
}

func ParseConfig(args []string, env map[string]string) (*Config, error) {
	config := &Config{
		AppNamespace:           envString(env, "MPP_APP_NS", "mpp-system"),
		RuntimeNamespace:       envString(env, "MPP_RUNTIME_NS", "mpp-browser-runtime"),
		ObservabilityNamespace: envString(env, "MPP_OBSERVABILITY_NS", "mpp-observability"),
		RolloutTimeout:         envInt(env, "MPP_SMOKE_ROLLOUT_TIMEOUT", 300),
		RequestTimeout:         envInt(env, "MPP_SMOKE_REQUEST_TIMEOUT", 10),
		CurlImage:              envString(env, "MPP_SMOKE_CURL_IMAGE", "curlimages/curl:8.11.1"),
		PublicURL:              env["MPP_PUBLIC_URL"],
		APIBaseURL:             env["MPP_API_BASE_URL"],
		AuthToken:              env["MPP_SMOKE_AUTH_TOKEN"],
		ProjectID:              env["MPP_SMOKE_PROJECT_ID"],
		BrowserPlatform:        envString(env, "MPP_SMOKE_BROWSER_PLATFORM", "douyin"),
		FullE2E:                truthy(env["MPP_SMOKE_FULL_E2E"]),
		RunUserFlowProbes:      truthy(env["MPP_SMOKE_RUN_USER_FLOW_PROBES"]),
		RunBrowserSessionProbe: truthy(env["MPP_SMOKE_RUN_BROWSER_SESSION_PROBE"]),
		RequireUserFlows:       truthy(env["MPP_SMOKE_REQUIRE_USER_FLOWS"]),
		SkipPublic:             truthy(env["MPP_SMOKE_SKIP_PUBLIC"]),
		SkipInternalHTTP:       truthy(env["MPP_SMOKE_SKIP_INTERNAL_HTTP"]),
		SkipRuntimeRBAC:        truthy(env["MPP_SMOKE_SKIP_RUNTIME_RBAC"]),
		SkipRuntimeCleanup:     truthy(env["MPP_SMOKE_SKIP_RUNTIME_CLEANUP"]),
		SkipDiagnostics:        truthy(env["MPP_SMOKE_SKIP_DIAGNOSTICS"]),
		DiagnosticLines:        envInt(env, "MPP_SMOKE_DIAGNOSTIC_LINES", 60),
		ReportJSON:             env["MPP_SMOKE_REPORT_JSON"],
		ReportJUnit:            env["MPP_SMOKE_REPORT_JUNIT"],
		Verbose:                truthy(env["MPP_SMOKE_VERBOSE"]),
	}

	flags := flag.NewFlagSet("smoke-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}

	flags.StringVar(&config.AppNamespace, "app-namespace", config.AppNamespace, "")
	flags.StringVar(&config.RuntimeNamespace, "runtime-namespace", config.RuntimeNamespace, "")
	flags.StringVar(&config.ObservabilityNamespace, "observability-namespace", config.ObservabilityNamespace, "")
	flags.IntVar(&config.RolloutTimeout, "rollout-timeout", config.RolloutTimeout, "")
	flags.IntVar(&config.RequestTimeout, "request-timeout", config.RequestTimeout, "")
	flags.StringVar(&config.CurlImage, "curl-image", config.CurlImage, "")
	flags.StringVar(&config.PublicURL, "public-url", config.PublicURL, "")
	flags.StringVar(&config.APIBaseURL, "api-base-url", config.APIBaseURL, "")
	flags.StringVar(&config.AuthToken, "auth-token", config.AuthToken, "")
	flags.StringVar(&config.ProjectID, "project-id", config.ProjectID, "")
	flags.StringVar(&config.BrowserPlatform, "browser-platform", config.BrowserPlatform, "")
	flags.BoolVar(&config.FullE2E, "full-e2e", config.FullE2E, "")
	flags.BoolVar(&config.RunUserFlowProbes, "run-user-flow-probes", config.RunUserFlowProbes, "")
	flags.BoolVar(&config.RunBrowserSessionProbe, "run-browser-session-probe", config.RunBrowserSessionProbe, "")
	flags.BoolVar(&config.RequireUserFlows, "require-user-flows", config.RequireUserFlows, "")
	flags.BoolVar(&config.SkipPublic, "skip-public", config.SkipPublic, "")
	flags.BoolVar(&config.SkipInternalHTTP, "skip-internal-http", config.SkipInternalHTTP, "")
	flags.BoolVar(&config.SkipRuntimeRBAC, "skip-runtime-rbac", config.SkipRuntimeRBAC, "")
	flags.BoolVar(&config.SkipRuntimeCleanup, "skip-runtime-cleanup", config.SkipRuntimeCleanup, "")
	flags.BoolVar(&config.SkipDiagnostics, "skip-diagnostics", config.SkipDiagnostics, "")
	flags.IntVar(&config.DiagnosticLines, "diagnostic-lines", config.DiagnosticLines, "")
	flags.StringVar(&config.ReportJSON, "report-json", config.ReportJSON, "")
	flags.StringVar(&config.ReportJUnit, "report-junit", config.ReportJUnit, "")
	flags.BoolVar(&config.DryRun, "dry-run", false, "")
	flags.BoolVar(&config.Verbose, "verbose", config.Verbose, "")
	flags.BoolVar(&config.Verbose, "v", config.Verbose, "")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, ErrHelp
		}
		return nil, err
	}
	if err := config.Normalize(); err != nil {
		return nil, err
	}
	return config, nil
}

func (config *Config) Normalize() error {
	var err error
	if config.AppNamespace, err = cleanRequired(config.AppNamespace, "app namespace"); err != nil {
		return err
	}
	if config.RuntimeNamespace, err = cleanRequired(config.RuntimeNamespace, "runtime namespace"); err != nil {
		return err
	}
	if config.ObservabilityNamespace, err = cleanRequired(config.ObservabilityNamespace, "observability namespace"); err != nil {
		return err
	}
	if config.CurlImage, err = cleanRequired(config.CurlImage, "curl image"); err != nil {
		return err
	}
	if config.BrowserPlatform, err = cleanRequired(config.BrowserPlatform, "browser platform"); err != nil {
		return err
	}
	if err := positiveInteger(config.RolloutTimeout, "rollout timeout"); err != nil {
		return err
	}
	if err := positiveInteger(config.RequestTimeout, "request timeout"); err != nil {
		return err
	}
	if err := positiveInteger(config.DiagnosticLines, "diagnostic lines"); err != nil {
		return err
	}
	if config.PublicURL, err = normalizeURL(config.PublicURL, "public URL"); err != nil {
		return err
	}
	if config.APIBaseURL, err = normalizeURL(config.APIBaseURL, "API base URL"); err != nil {
		return err
	}
	if config.APIBaseURL == "" {
		config.APIBaseURL = config.PublicURL
	}
	config.AuthToken = blankToEmpty(config.AuthToken)
	config.ProjectID = blankToEmpty(config.ProjectID)
	config.ReportJSON = blankToEmpty(config.ReportJSON)
	config.ReportJUnit = blankToEmpty(config.ReportJUnit)
	if config.FullE2E {
		config.RunUserFlowProbes = true
		config.RunBrowserSessionProbe = true
		config.RequireUserFlows = true
	}
	if config.RunBrowserSessionProbe {
		config.RunUserFlowProbes = true
	}
	if config.FullE2E {
		if config.SkipPublic || config.SkipInternalHTTP || config.SkipRuntimeRBAC || config.SkipRuntimeCleanup {
			return fmt.Errorf("full E2E cannot be combined with core smoke skip flags")
		}
		if config.PublicURL == "" {
			return fmt.Errorf("full E2E requires --public-url or MPP_PUBLIC_URL")
		}
		if config.AuthToken == "" {
			return fmt.Errorf("full E2E requires --auth-token or MPP_SMOKE_AUTH_TOKEN")
		}
		if config.ProjectID == "" {
			return fmt.Errorf("full E2E requires --project-id or MPP_SMOKE_PROJECT_ID")
		}
	}
	return nil
}

func (config *Config) PublicURLConfigured() bool {
	return config.PublicURL != ""
}

func (config *Config) APIBaseURLConfigured() bool {
	return config.APIBaseURL != ""
}

func (config *Config) AuthConfigured() bool {
	return config.AuthToken != ""
}

func (config *Config) ProjectConfigured() bool {
	return config.ProjectID != ""
}

func (config *Config) UserFlowInputsConfigured() bool {
	return config.APIBaseURLConfigured() && config.AuthConfigured()
}

func (config *Config) CheckSettings() checks.Settings {
	return checks.Settings{
		AppNamespace:           config.AppNamespace,
		RuntimeNamespace:       config.RuntimeNamespace,
		ObservabilityNamespace: config.ObservabilityNamespace,
		RolloutTimeout:         config.RolloutTimeout,
		RequestTimeout:         config.RequestTimeout,
		CurlImage:              config.CurlImage,
		PublicURL:              config.PublicURL,
		APIBaseURL:             config.APIBaseURL,
		AuthToken:              config.AuthToken,
		ProjectID:              config.ProjectID,
		BrowserPlatform:        config.BrowserPlatform,
		RunUserFlowProbes:      config.RunUserFlowProbes,
		RunBrowserSessionProbe: config.RunBrowserSessionProbe,
		RequireUserFlows:       config.RequireUserFlows,
		SkipPublic:             config.SkipPublic,
		SkipInternalHTTP:       config.SkipInternalHTTP,
		SkipRuntimeRBAC:        config.SkipRuntimeRBAC,
		SkipRuntimeCleanup:     config.SkipRuntimeCleanup,
		DryRun:                 config.DryRun,
		Verbose:                config.Verbose,
	}
}

func envString(env map[string]string, key string, fallback string) string {
	if value, ok := env[key]; ok {
		return value
	}
	return fallback
}

func envInt(env map[string]string, key string, fallback int) int {
	value := strings.TrimSpace(env[key])
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func blankToEmpty(value string) string {
	return strings.TrimSpace(value)
}

func cleanRequired(value string, label string) (string, error) {
	text := blankToEmpty(value)
	if text == "" {
		return "", fmt.Errorf("%s must not be empty", label)
	}
	return text, nil
}

func positiveInteger(value int, label string) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", label)
	}
	return nil
}

func normalizeURL(value string, label string) (string, error) {
	text := blankToEmpty(value)
	if text == "" {
		return "", nil
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return "", fmt.Errorf("%s must be a valid URL", label)
	}
	if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%s must be an http or https URL", label)
	}
	return strings.TrimRight(text, "/"), nil
}

func usage() string {
	return `Usage: smoke-test [options]

Cluster scope:
  --app-namespace NAME             Application namespace. Default: mpp-system
  --runtime-namespace NAME         Browser runtime namespace. Default: mpp-browser-runtime
  --observability-namespace NAME   Observability namespace. Default: mpp-observability
  --rollout-timeout SECONDS        Rollout timeout per Deployment. Default: 300
  --request-timeout SECONDS        HTTP request timeout. Default: 10
  --curl-image IMAGE               Image used for in-cluster HTTP probes. Default: curlimages/curl:8.11.1

Public and user-flow probes:
  --public-url URL                 Public frontend base URL. Env: MPP_PUBLIC_URL
  --api-base-url URL               API base URL. Defaults to --public-url. Env: MPP_API_BASE_URL
  --auth-token TOKEN               Bearer token for authenticated smoke probes. Env: MPP_SMOKE_AUTH_TOKEN
  --project-id ID                  Existing project ID for collaboration and publishing dependency probes. Env: MPP_SMOKE_PROJECT_ID
  --browser-platform NAME          Browser session platform. Default: douyin
  --full-e2e                       Require public, authenticated, project, collaboration, and browser-session probes.
  --run-user-flow-probes           Run authenticated read and project-scoped probes.
  --run-browser-session-probe      Start and cancel a remote browser session through the backend API.
  --require-user-flows             Fail instead of skipping when user-flow inputs are missing.

Skips:
  --skip-public                    Skip public URL probes.
  --skip-internal-http             Skip in-cluster HTTP probes.
  --skip-runtime-rbac              Skip browser runtime RBAC can-i probes.
  --skip-runtime-cleanup           Skip runtime Pod cleanup-state probes.
  --skip-diagnostics               Skip failure diagnostics collection.
  --diagnostic-lines LINES         Tail lines to print per diagnostic command. Default: 60

Execution:
  --dry-run                        Print command intent without calling kubectl.
  --report-json PATH               Write a machine-readable JSON report. Env: MPP_SMOKE_REPORT_JSON
  --report-junit PATH              Write a JUnit XML report. Env: MPP_SMOKE_REPORT_JUNIT
  -v, --verbose                    Print command details.
  -h, --help                       Show this help.

Examples:
  go run . --public-url https://mpp.example.com
  MPP_SMOKE_AUTH_TOKEN=... go run . --public-url https://mpp.example.com --run-user-flow-probes
  MPP_SMOKE_AUTH_TOKEN=... go run . --public-url https://mpp.example.com --project-id ... --full-e2e
  go run . --run-browser-session-probe --auth-token ... --public-url https://mpp.example.com
`
}
