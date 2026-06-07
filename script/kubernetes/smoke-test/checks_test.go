package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSecretCheckFailsWhenRequiredSecretKeysAreEmpty(t *testing.T) {
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, fakeKubectl{secretData: Object{}}, fakeHTTP{})

	suite.configuration()

	if !hasResult(reporter.Failures, "mpp-app-secrets keys") {
		t.Fatalf("expected mpp-app-secrets keys failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "mpp-app-secrets keys")
	if !strings.Contains(detail, "JWT_SECRET") {
		t.Fatalf("expected missing JWT_SECRET detail, got %q", detail)
	}
}

func TestSecretCheckPassesWhenRequiredSecretKeysArePopulated(t *testing.T) {
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, fakeKubectl{secretData: requiredSecretData()}, fakeHTTP{})

	suite.configuration()

	if hasResult(reporter.Failures, "mpp-app-secrets keys") {
		t.Fatalf("did not expect mpp-app-secrets keys failure, got %#v", reporter.Failures)
	}
}

func TestBrowserSessionProbeFailureIsRequired(t *testing.T) {
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, fakeKubectl{secretData: requiredSecretData()}, fakeHTTP{
		postResponse: Response{Status: 500, Body: "failed to start"},
	})

	suite.browserSessionProbe()

	if !hasResult(reporter.Failures, "remote browser session lifecycle") {
		t.Fatalf("expected browser session failure, got %#v", reporter.Failures)
	}
	if len(reporter.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", reporter.Warnings)
	}
}

func TestAuthenticatedProbeRejectsFrontendFallbackBody(t *testing.T) {
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, fakeKubectl{secretData: requiredSecretData()}, fakeHTTP{
		getResponse: Response{Status: 200, Body: "<html>login</html>"},
	})
	suite.config.RunUserFlowProbes = true

	suite.authenticatedUserFlows()

	if !hasResult(reporter.Failures, "authenticated dashboard session") {
		t.Fatalf("expected authenticated dashboard failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "authenticated dashboard session")
	if !strings.Contains(detail, "non-JSON") {
		t.Fatalf("expected non-JSON failure detail, got %q", detail)
	}
}

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

func suiteWith(t *testing.T, reporter *Reporter, kubectl KubernetesClient, http HTTPDoer) *Suite {
	t.Helper()
	config, err := ParseConfig(
		[]string{
			"--public-url",
			"https://mpp.example.com",
			"--auth-token",
			"smoke-token",
		},
		map[string]string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return NewSuite(config, kubectl, reporter, http)
}

type fakeKubectl struct {
	secretData Object
}

func (kubectl fakeKubectl) CurrentContext() (string, error) {
	return "test-context", nil
}

func (kubectl fakeKubectl) ClientVersion() (any, error) {
	return Object{"clientVersion": Object{"gitVersion": "test-version"}}, nil
}

func (kubectl fakeKubectl) Namespace(name string) (Object, error) {
	return Object{"metadata": Object{"name": name, "labels": Object{}}}, nil
}

func (kubectl fakeKubectl) Resource(kind string, name string, namespace string) (Object, error) {
	switch {
	case kind == "configmap" && name == "mpp-app-config" && namespace == "mpp-system":
		return Object{"data": requiredConfigData()}, nil
	case kind == "secret" && name == "mpp-app-secrets" && namespace == "mpp-system":
		return Object{"data": kubectl.secretData}, nil
	default:
		return Object{}, nil
	}
}

func (kubectl fakeKubectl) ResourceList(kind string, namespace string, selector string) ([]Object, error) {
	return nil, nil
}

func (kubectl fakeKubectl) RolloutStatus(resource string, namespace string, timeout int) (string, error) {
	return "ok", nil
}

func (kubectl fakeKubectl) AuthCanI(verb string, resource string, asUser string, namespace string) (string, error) {
	return "yes", nil
}

func (kubectl fakeKubectl) Exec(resource string, command []string, namespace string, container string) (string, error) {
	return "ready", nil
}

func (kubectl fakeKubectl) CurlFromEphemeralPod(namespace string, image string, targetURL string, timeout int, headers map[string]string, method string, body string) (string, error) {
	return `{"status":"ready"}`, nil
}

type fakeHTTP struct {
	getResponse  Response
	postResponse Response
}

func (http fakeHTTP) Get(targetURL string, headers map[string]string) (Response, error) {
	if http.getResponse.Status != 0 {
		return http.getResponse, nil
	}
	return Response{Status: 200, Body: `{"status":"ready"}`}, nil
}

func (http fakeHTTP) Post(targetURL string, headers map[string]string, jsonBody any) (Response, error) {
	if http.postResponse.Status != 0 {
		return http.postResponse, nil
	}
	return Response{Status: 201, Body: `{"session_id":"session-1"}`}, nil
}

func (http fakeHTTP) Delete(targetURL string, headers map[string]string) (Response, error) {
	return Response{Status: 200, Body: `{"status":"expired"}`}, nil
}

func requiredConfigData() Object {
	data := make(Object, len(requiredConfigKeys))
	for _, key := range requiredConfigKeys {
		data[key] = "configured-value"
	}
	return data
}

func requiredSecretData() Object {
	data := make(Object, len(requiredSecretKeys))
	for _, key := range requiredSecretKeys {
		data[key] = "encoded-value"
	}
	return data
}

func hasResult(results []CheckResult, name string) bool {
	for _, result := range results {
		if result.Name == name {
			return true
		}
	}
	return false
}

func resultDetail(results []CheckResult, name string) string {
	for _, result := range results {
		if result.Name == name {
			return result.Detail
		}
	}
	return ""
}
