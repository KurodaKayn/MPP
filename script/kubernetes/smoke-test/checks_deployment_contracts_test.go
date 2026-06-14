package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeploymentContractsPassWithPinnedKubernetesResources(t *testing.T) {
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, contractFakeKubectl(), fakeHTTP{})

	suite.deploymentContracts()

	if len(reporter.Failures) != 0 {
		t.Fatalf("expected deployment contracts to pass, got %#v", reporter.Failures)
	}
}

func TestGatewayContractFailsWhenCollabRouteIsMissing(t *testing.T) {
	kubectl := contractFakeKubectl()
	kubectl.resources[fakeResourceKey("ingress", publicGatewayName, "mpp-system")] = ingressWithoutCollabRoute()
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.gatewayContract()

	if !hasResult(reporter.Failures, "public Ingress route contract") {
		t.Fatalf("expected public Ingress route contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "public Ingress route contract")
	if !strings.Contains(detail, "/collab") {
		t.Fatalf("expected missing /collab detail, got %q", detail)
	}
}

func TestGatewayContractFailsWhenTLSHostDoesNotMatchPublicURL(t *testing.T) {
	kubectl := contractFakeKubectl()
	ingress := dryRunIngress(publicGatewayName)
	tls := asObjectSlice(dig(ingress, "spec", "tls"))
	tls[0]["hosts"] = []any{"wrong.example.com"}
	kubectl.resources[fakeResourceKey("ingress", publicGatewayName, "mpp-system")] = ingress
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.gatewayContract()

	if !hasResult(reporter.Failures, "public Ingress route contract") {
		t.Fatalf("expected public Ingress route contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "public Ingress route contract")
	if !strings.Contains(detail, "does not cover mpp.example.com") {
		t.Fatalf("expected TLS host mismatch detail, got %q", detail)
	}
}

func TestAppNetworkPolicyContractFailsWhenPublishWorkerCallerIsMissing(t *testing.T) {
	kubectl := contractFakeKubectl()
	policies := dryRunNetworkPolicies("mpp-system")
	for index, policy := range policies {
		if stringValue(dig(policy, "metadata", "name")) == "browser-worker-internal-access" {
			policies[index] = internalNetworkPolicy("browser-worker-internal-access", "browser-worker", 8081, "backend")
		}
	}
	kubectl.lists[fakeResourceKey("networkpolicy", "", "mpp-system")] = policies
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.appNetworkPolicyContract()

	if !hasResult(reporter.Failures, "app namespace NetworkPolicy contract") {
		t.Fatalf("expected NetworkPolicy contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "app namespace NetworkPolicy contract")
	if !strings.Contains(detail, "publish-worker") {
		t.Fatalf("expected missing publish-worker detail, got %q", detail)
	}
}

func TestAppNetworkPolicyContractFailsWhenPublicIngressNamespaceIsMissing(t *testing.T) {
	kubectl := contractFakeKubectl()
	policies := dryRunNetworkPolicies("mpp-system")
	for index, policy := range policies {
		if stringValue(dig(policy, "metadata", "name")) == "public-frontend-access" {
			policies[index] = Object{
				"metadata": Object{"name": "public-frontend-access"},
				"spec": Object{
					"podSelector": Object{"matchLabels": Object{"app.kubernetes.io/component": "frontend"}},
					"policyTypes": []any{"Ingress"},
					"ingress": []Object{
						{
							"from": []Object{
								{"namespaceSelector": Object{"matchLabels": Object{"mpp.kurodakayn.dev/public-ingress": "false"}}},
							},
							"ports": []Object{{"port": 3000}},
						},
					},
				},
			}
		}
	}
	kubectl.lists[fakeResourceKey("networkpolicy", "", "mpp-system")] = policies
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.appNetworkPolicyContract()

	if !hasResult(reporter.Failures, "app namespace NetworkPolicy contract") {
		t.Fatalf("expected NetworkPolicy contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "app namespace NetworkPolicy contract")
	if !strings.Contains(detail, "public ingress namespace") {
		t.Fatalf("expected public ingress namespace detail, got %q", detail)
	}
}

func TestBrowserWorkerRuntimeContractRejectsDockerSocketMount(t *testing.T) {
	kubectl := contractFakeKubectl()
	deployment := browserWorkerDeployment()
	templateSpec := asObject(dig(deployment, "spec", "template", "spec"))
	templateSpec["volumes"] = []Object{
		{
			"name":     "docker-sock",
			"hostPath": Object{"path": "/var/run/docker.sock"},
		},
	}
	kubectl.resources[fakeResourceKey("deployment", browserWorkerDeploymentName, "mpp-system")] = deployment
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.browserWorkerRuntimeContract()

	if !hasResult(reporter.Failures, "browser-worker Kubernetes runtime contract") {
		t.Fatalf("expected browser-worker runtime contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "browser-worker Kubernetes runtime contract")
	if !strings.Contains(detail, "docker.sock") {
		t.Fatalf("expected docker.sock detail, got %q", detail)
	}
}

func TestBrowserWorkerRuntimeContractRequiresKubernetesDriver(t *testing.T) {
	kubectl := contractFakeKubectl()
	deployment := browserWorkerDeployment()
	setContainerEnv(deployment, "browser-worker", "BROWSER_RUNTIME_DRIVER", "docker")
	kubectl.resources[fakeResourceKey("deployment", browserWorkerDeploymentName, "mpp-system")] = deployment
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.browserWorkerRuntimeContract()

	if !hasResult(reporter.Failures, "browser-worker Kubernetes runtime contract") {
		t.Fatalf("expected browser-worker runtime contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "browser-worker Kubernetes runtime contract")
	if !strings.Contains(detail, "runtime driver mismatch") {
		t.Fatalf("expected driver mismatch detail, got %q", detail)
	}
}

func TestBrowserWorkerRuntimeContractRequiresPinnedRuntimeImage(t *testing.T) {
	kubectl := contractFakeKubectl()
	deployment := browserWorkerDeployment()
	setContainerEnv(deployment, "browser-worker", "BROWSER_RUNTIME_IMAGE", "registry.example.invalid/kurodakayn/mpp-browser-runtime:replace-me")
	kubectl.resources[fakeResourceKey("deployment", browserWorkerDeploymentName, "mpp-system")] = deployment
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.browserWorkerRuntimeContract()

	if !hasResult(reporter.Failures, "browser-worker Kubernetes runtime contract") {
		t.Fatalf("expected browser-worker runtime contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "browser-worker Kubernetes runtime contract")
	if !strings.Contains(detail, "BROWSER_RUNTIME_IMAGE") {
		t.Fatalf("expected runtime image detail, got %q", detail)
	}
}

func TestRuntimeAdmissionPolicyContractFailsWhenBindingDoesNotDeny(t *testing.T) {
	kubectl := contractFakeKubectl()
	binding := dryRunAdmissionPolicyBinding(runtimeAdmissionPolicyName)
	spec := asObject(binding["spec"])
	spec["validationActions"] = []any{"Warn"}
	kubectl.resources[fakeResourceKey("validatingadmissionpolicybinding", runtimeAdmissionPolicyName, "")] = binding
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.runtimeAdmissionPolicyContract()

	if !hasResult(reporter.Failures, "browser runtime admission contract") {
		t.Fatalf("expected runtime admission contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "browser runtime admission contract")
	if !strings.Contains(detail, "deny invalid Pods") {
		t.Fatalf("expected Deny action detail, got %q", detail)
	}
}

func TestRuntimeAdmissionPolicyContractFailsWhenResourceValidationIsMissing(t *testing.T) {
	kubectl := contractFakeKubectl()
	policy := dryRunAdmissionPolicy(runtimeAdmissionPolicyName)
	spec := asObject(policy["spec"])
	spec["validations"] = []Object{
		{"expression": "object.metadata.name.startsWith('mpp-browser-')"},
		{"expression": "object.spec.restartPolicy == 'Never'"},
	}
	kubectl.resources[fakeResourceKey("validatingadmissionpolicy", runtimeAdmissionPolicyName, "")] = policy
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.runtimeAdmissionPolicyContract()

	if !hasResult(reporter.Failures, "browser runtime admission contract") {
		t.Fatalf("expected runtime admission contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "browser runtime admission contract")
	if !strings.Contains(detail, "has(c.resources.requests)") {
		t.Fatalf("expected resource validation detail, got %q", detail)
	}
}

func TestRuntimePodSecurityContractPassesWhenNoActiveRuntimePodsExist(t *testing.T) {
	kubectl := contractFakeKubectl()
	kubectl.lists[fakeResourceKey("pods", runtimePodSelector, "mpp-browser-runtime")] = nil
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.runtimePodSecurityContract()

	if len(reporter.Failures) != 0 {
		t.Fatalf("expected runtime Pod security contract to pass, got %#v", reporter.Failures)
	}
	if !hasResult(reporter.Passes, "runtime Pod security contract") {
		t.Fatalf("expected runtime Pod security contract pass, got %#v", reporter.Passes)
	}
}

func TestRuntimePodSecurityContractRejectsPrivilegedRuntimePod(t *testing.T) {
	kubectl := contractFakeKubectl()
	pod := runtimePod()
	container := asObjectSlice(dig(pod, "spec", "containers"))[0]
	securityContext := asObject(container["securityContext"])
	securityContext["allowPrivilegeEscalation"] = true
	kubectl.lists[fakeResourceKey("pods", runtimePodSelector, "mpp-browser-runtime")] = []Object{pod}
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.runtimePodSecurityContract()

	if !hasResult(reporter.Failures, "runtime Pod security contract") {
		t.Fatalf("expected runtime Pod security contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "runtime Pod security contract")
	if !strings.Contains(detail, "allowPrivilegeEscalation") {
		t.Fatalf("expected allowPrivilegeEscalation detail, got %q", detail)
	}
}

func TestRuntimePodSecurityContractRejectsMissingRuntimePorts(t *testing.T) {
	kubectl := contractFakeKubectl()
	pod := runtimePod()
	container := asObjectSlice(dig(pod, "spec", "containers"))[0]
	container["ports"] = []Object{{"name": "cdp", "containerPort": 9222}}
	kubectl.lists[fakeResourceKey("pods", runtimePodSelector, "mpp-browser-runtime")] = []Object{pod}
	reporter := NewReporter(&bytes.Buffer{}, false)
	suite := suiteWith(t, reporter, kubectl, fakeHTTP{})

	suite.runtimePodSecurityContract()

	if !hasResult(reporter.Failures, "runtime Pod security contract") {
		t.Fatalf("expected runtime Pod security contract failure, got %#v", reporter.Failures)
	}
	detail := resultDetail(reporter.Failures, "runtime Pod security contract")
	if !strings.Contains(detail, "6080") {
		t.Fatalf("expected missing stream port detail, got %q", detail)
	}
}

func contractFakeKubectl() fakeKubectl {
	return fakeKubectl{
		secretData: requiredSecretData(),
		resources: map[string]Object{
			fakeResourceKey("ingress", publicGatewayName, "mpp-system"):                         dryRunIngress(publicGatewayName),
			fakeResourceKey("deployment", browserWorkerDeploymentName, "mpp-system"):            browserWorkerDeployment(),
			fakeResourceKey("validatingadmissionpolicy", runtimeAdmissionPolicyName, ""):        dryRunAdmissionPolicy(runtimeAdmissionPolicyName),
			fakeResourceKey("validatingadmissionpolicybinding", runtimeAdmissionPolicyName, ""): dryRunAdmissionPolicyBinding(runtimeAdmissionPolicyName),
			fakeResourceKey("configmap", "mpp-app-config", "mpp-system"):                        Object{"data": requiredConfigData()},
			fakeResourceKey("secret", "mpp-app-secrets", "mpp-system"):                          Object{"data": requiredSecretData()},
			fakeResourceKey("serviceaccount", "browser-worker-runtime-manager", "mpp-system"):   Object{"metadata": Object{"name": "browser-worker-runtime-manager"}},
		},
		lists: map[string][]Object{
			fakeResourceKey("networkpolicy", "", "mpp-system"):                 dryRunNetworkPolicies("mpp-system"),
			fakeResourceKey("networkpolicy", "", "mpp-browser-runtime"):        dryRunNetworkPolicies("mpp-browser-runtime"),
			fakeResourceKey("pods", runtimePodSelector, "mpp-browser-runtime"): []Object{runtimePod()},
			fakeResourceKey("pods", "", "mpp-system"):                          []Object{readyAppPod()},
			fakeResourceKey("deployments", "", "mpp-system"):                   (&Kubectl{}).dryRunDeployments(),
		},
	}
}

func browserWorkerDeployment() Object {
	return (&Kubectl{}).dryRunDeployment(browserWorkerDeploymentName)
}

func runtimePod() Object {
	return (&Kubectl{}).dryRunPods(runtimePodSelector)[0]
}

func readyAppPod() Object {
	return Object{
		"metadata": Object{"name": "mpp-app-test"},
		"status": Object{
			"conditions": []Object{{"type": "Ready", "status": "True"}},
		},
	}
}

func ingressWithoutCollabRoute() Object {
	ingress := dryRunIngress(publicGatewayName)
	rules := asObjectSlice(dig(ingress, "spec", "rules"))
	httpRule := asObject(rules[0]["http"])
	httpRule["paths"] = []Object{
		{"path": "/", "pathType": "Prefix", "backend": Object{"service": Object{"name": "frontend"}}},
	}
	return ingress
}

func setContainerEnv(deployment Object, containerName string, key string, value string) {
	container, ok := namedContainer(deployment, containerName)
	if !ok {
		return
	}
	envVars := asObjectSlice(container["env"])
	for _, envVar := range envVars {
		if stringValue(envVar["name"]) == key {
			envVar["value"] = value
			return
		}
	}
	container["env"] = append(envVars, Object{"name": key, "value": value})
}
