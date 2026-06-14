package main

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	publicGatewayName           = "mpp-public-gateway"
	browserWorkerDeploymentName = "browser-worker"
	runtimeAdmissionPolicyName  = "mpp-browser-runtime-pods"
)

var requiredAppNetworkPolicies = []string{
	"mpp-system-default-deny",
	"public-frontend-access",
	"public-collab-access",
	"frontend-backend-access",
	"browser-worker-internal-access",
	"ai-service-internal-access",
	"content-pipeline-internal-access",
	"collab-service-internal-access",
}

var runtimeResourceEnvKeys = []string{
	"BROWSER_RUNTIME_KUBERNETES_CPU_REQUEST",
	"BROWSER_RUNTIME_KUBERNETES_CPU_LIMIT",
	"BROWSER_RUNTIME_KUBERNETES_MEMORY_REQUEST",
	"BROWSER_RUNTIME_KUBERNETES_MEMORY_LIMIT",
}

func (suite *Suite) deploymentContracts() {
	suite.reporter.Section("Deployment Contracts")
	suite.gatewayContract()
	suite.appNetworkPolicyContract()
	suite.browserWorkerRuntimeContract()
	suite.runtimeAdmissionPolicyContract()
}

func (suite *Suite) gatewayContract() {
	suite.check("public Ingress route contract", true, func() (string, error) {
		ingress, err := suite.kubectl.Resource("ingress", publicGatewayName, suite.config.AppNamespace)
		if err != nil {
			return "", err
		}
		if err := assertEqual(publicGatewayName, stringValue(dig(ingress, "metadata", "name")), "missing public Ingress"); err != nil {
			return "", err
		}
		if err := assertPresent(dig(ingress, "spec", "ingressClassName"), "public Ingress is missing ingressClassName"); err != nil {
			return "", err
		}
		if err := assertIngressRoute(ingress, "/collab", "collab-service"); err != nil {
			return "", err
		}
		if err := assertIngressRoute(ingress, "/", "frontend"); err != nil {
			return "", err
		}
		if err := assertIngressTLS(ingress, suite.config.PublicURL); err != nil {
			return "", err
		}
		return "routes=/collab->collab-service,/->frontend", nil
	})
}

func (suite *Suite) appNetworkPolicyContract() {
	suite.check("app namespace NetworkPolicy contract", true, func() (string, error) {
		policies, err := suite.kubectl.ResourceList("networkpolicy", suite.config.AppNamespace, "")
		if err != nil {
			return "", err
		}
		missing := missingNamedResources(policies, requiredAppNetworkPolicies)
		if err := assert(len(missing) == 0, "missing NetworkPolicies: "+strings.Join(missing, ", ")); err != nil {
			return "", err
		}
		byName := resourcesByName(policies)
		if err := assertDefaultDenyPolicy(byName["mpp-system-default-deny"]); err != nil {
			return "", err
		}
		publicChecks := []struct {
			name      string
			component string
			port      int
		}{
			{name: "public-frontend-access", component: "frontend", port: 3000},
			{name: "public-collab-access", component: "collab-service", port: 8090},
		}
		for _, check := range publicChecks {
			if err := assertPublicIngressPolicy(byName[check.name], check.component, check.port); err != nil {
				return "", failure("%s: %s", check.name, err)
			}
		}
		internalChecks := []struct {
			name        string
			component   string
			port        int
			callers     []string
			description string
		}{
			{name: "frontend-backend-access", component: "backend", port: 8080, callers: []string{"frontend", "content-pipeline-service"}},
			{name: "browser-worker-internal-access", component: "browser-worker", port: 8081, callers: []string{"backend", "publish-worker"}},
			{name: "ai-service-internal-access", component: "ai-service", port: 8000, callers: []string{"backend", "publish-worker"}},
			{name: "content-pipeline-internal-access", component: "content-pipeline-service", port: 50051, callers: []string{"backend", "publish-worker"}},
			{name: "collab-service-internal-access", component: "collab-service", port: 8090, callers: []string{"backend", "publish-worker"}},
		}
		for _, check := range internalChecks {
			if err := assertInternalIngressPolicy(byName[check.name], check.component, check.port, check.callers); err != nil {
				return "", failure("%s: %s", check.name, err)
			}
		}
		return fmt.Sprintf("%d NetworkPolicies checked", len(requiredAppNetworkPolicies)), nil
	})
}

func (suite *Suite) browserWorkerRuntimeContract() {
	suite.check("browser-worker Kubernetes runtime contract", true, func() (string, error) {
		deployment, err := suite.kubectl.Resource("deployment", browserWorkerDeploymentName, suite.config.AppNamespace)
		if err != nil {
			return "", err
		}
		templateSpec := asObject(dig(deployment, "spec", "template", "spec"))
		if err := assertEqual("browser-worker-runtime-manager", stringValue(templateSpec["serviceAccountName"]), "browser-worker ServiceAccount mismatch"); err != nil {
			return "", err
		}
		if err := assertNoDockerSocket(deployment); err != nil {
			return "", err
		}
		container, ok := namedContainer(deployment, "browser-worker")
		if err := assert(ok, "browser-worker container is missing"); err != nil {
			return "", err
		}
		env := containerEnv(container)
		if err := assertEqual("kubernetes", env["BROWSER_RUNTIME_DRIVER"], "browser runtime driver mismatch"); err != nil {
			return "", err
		}
		if err := assertEqual(suite.config.RuntimeNamespace, env["BROWSER_RUNTIME_KUBERNETES_NAMESPACE"], "runtime namespace mismatch"); err != nil {
			return "", err
		}
		runtimeImage := strings.TrimSpace(env["BROWSER_RUNTIME_IMAGE"])
		if err := assert(runtimeImage != "", "BROWSER_RUNTIME_IMAGE is missing"); err != nil {
			return "", err
		}
		if err := assert(!unresolvedImage(runtimeImage), "BROWSER_RUNTIME_IMAGE is not pinned for this environment: "+runtimeImage); err != nil {
			return "", err
		}
		missing := make([]string, 0)
		for _, key := range runtimeResourceEnvKeys {
			if strings.TrimSpace(env[key]) == "" {
				missing = append(missing, key)
			}
		}
		if err := assert(len(missing) == 0, "missing runtime resource env keys: "+strings.Join(missing, ", ")); err != nil {
			return "", err
		}
		if err := assertContainerRestricted(container); err != nil {
			return "", failure("browser-worker container security: %s", err)
		}
		return "driver=kubernetes runtimeImage=" + runtimeImage, nil
	})
}

func (suite *Suite) runtimeAdmissionPolicyContract() {
	suite.check("browser runtime admission contract", true, func() (string, error) {
		policy, err := suite.kubectl.Resource("validatingadmissionpolicy", runtimeAdmissionPolicyName, "")
		if err != nil {
			return "", err
		}
		if err := assertEqual("Fail", stringValue(dig(policy, "spec", "failurePolicy")), "runtime admission failurePolicy mismatch"); err != nil {
			return "", err
		}
		expressions := validationExpressions(policy)
		requiredSnippets := []string{
			"object.metadata.name.startsWith('mpp-browser-')",
			"object.spec.restartPolicy == 'Never'",
			"object.spec.automountServiceAccountToken == false",
			"object.spec.containers.size() == 1",
			"object.spec.containers[0].ports.exists(port, port.containerPort == 9222)",
			"object.spec.containers[0].ports.exists(port, port.containerPort == 6080)",
			"has(c.resources.requests)",
			"allowPrivilegeEscalation) && c.securityContext.allowPrivilegeEscalation == false",
		}
		missing := missingExpressionSnippets(expressions, requiredSnippets)
		if err := assert(len(missing) == 0, "admission policy missing expressions: "+strings.Join(missing, ", ")); err != nil {
			return "", err
		}
		binding, err := suite.kubectl.Resource("validatingadmissionpolicybinding", runtimeAdmissionPolicyName, "")
		if err != nil {
			return "", err
		}
		if err := assert(sliceContainsString(asSlice(dig(binding, "spec", "validationActions")), "Deny"), "runtime admission binding must deny invalid Pods"); err != nil {
			return "", err
		}
		selectorValue := stringValue(dig(
			binding,
			"spec",
			"matchResources",
			"namespaceSelector",
			"matchLabels",
			"mpp.kurodakayn.dev/browser-runtime-namespace",
		))
		if err := assertEqual("true", selectorValue, "runtime admission binding namespace selector mismatch"); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d validation expressions checked", len(requiredSnippets)), nil
	})
}

func (suite *Suite) runtimePodSecurityContract() {
	suite.check("runtime Pod security contract", true, func() (string, error) {
		pods, err := suite.kubectl.ResourceList("pods", suite.config.RuntimeNamespace, runtimePodSelector)
		if err != nil {
			return "", err
		}
		if len(pods) == 0 {
			return "no active runtime Pods to inspect", nil
		}
		violations := make([]string, 0)
		for _, pod := range pods {
			if err := assertRuntimePodSpec(pod); err != nil {
				violations = append(violations, fmt.Sprintf("%s: %s", stringValue(dig(pod, "metadata", "name")), err))
			}
		}
		if err := assert(len(violations) == 0, strings.Join(violations, "; ")); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d active runtime Pod specs checked", len(pods)), nil
	})
}

func assertIngressRoute(ingress Object, path string, serviceName string) error {
	for _, rule := range asSlice(dig(ingress, "spec", "rules")) {
		for _, route := range asSlice(dig(rule, "http", "paths")) {
			routeObject := asObject(route)
			if stringValue(routeObject["path"]) != path {
				continue
			}
			backendService := stringValue(dig(routeObject, "backend", "service", "name"))
			if backendService == serviceName {
				return nil
			}
			return failure("path %s targets %q instead of %q", path, backendService, serviceName)
		}
	}
	return failure("Ingress route %s -> %s is missing", path, serviceName)
}

func assertIngressTLS(ingress Object, publicURL string) error {
	tlsEntries := asSlice(dig(ingress, "spec", "tls"))
	if len(tlsEntries) == 0 {
		return CheckFailure("public Ingress is missing TLS")
	}
	expectedHost := hostFromURL(publicURL)
	for _, entry := range tlsEntries {
		hosts := asSlice(dig(entry, "hosts"))
		for _, rawHost := range hosts {
			host := stringValue(rawHost)
			if placeholderValue(host) {
				return failure("public Ingress TLS host is unresolved: %s", host)
			}
			if expectedHost != "" && host == expectedHost {
				return nil
			}
		}
		if expectedHost == "" && strings.TrimSpace(stringValue(dig(entry, "secretName"))) != "" {
			return nil
		}
	}
	if expectedHost != "" {
		return failure("public Ingress TLS does not cover %s", expectedHost)
	}
	return CheckFailure("public Ingress TLS is missing secretName")
}

func hostFromURL(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func assertDefaultDenyPolicy(policy Object) error {
	if err := assertEmptyPodSelector(policy); err != nil {
		return err
	}
	if err := assertPolicyType(policy, "Ingress"); err != nil {
		return err
	}
	if len(asSlice(dig(policy, "spec", "ingress"))) != 0 {
		return CheckFailure("default deny policy must not define ingress allow rules")
	}
	return nil
}

func assertPublicIngressPolicy(policy Object, component string, port int) error {
	if err := assertPolicyTargetsComponent(policy, component); err != nil {
		return err
	}
	if err := assertPolicyType(policy, "Ingress"); err != nil {
		return err
	}
	if err := assertPolicyHasPort(policy, port); err != nil {
		return err
	}
	if !policyAllowsNamespaceLabel(policy, "mpp.kurodakayn.dev/public-ingress", "true") {
		return CheckFailure("policy does not allow the labeled public ingress namespace")
	}
	return nil
}

func assertInternalIngressPolicy(policy Object, component string, port int, callers []string) error {
	if err := assertPolicyTargetsComponent(policy, component); err != nil {
		return err
	}
	if err := assertPolicyType(policy, "Ingress"); err != nil {
		return err
	}
	if err := assertPolicyHasPort(policy, port); err != nil {
		return err
	}
	missingCallers := make([]string, 0)
	for _, caller := range callers {
		if !policyAllowsPodComponent(policy, caller) {
			missingCallers = append(missingCallers, caller)
		}
	}
	if len(missingCallers) > 0 {
		return failure("policy does not allow callers: %s", strings.Join(missingCallers, ", "))
	}
	return nil
}

func assertEmptyPodSelector(policy Object) error {
	selector := asObject(dig(policy, "spec", "podSelector"))
	if len(selector) != 0 {
		return CheckFailure("podSelector must be empty")
	}
	return nil
}

func assertPolicyTargetsComponent(policy Object, component string) error {
	value := stringValue(dig(policy, "spec", "podSelector", "matchLabels", "app.kubernetes.io/component"))
	if value != component {
		return failure("policy targets component %q instead of %q", value, component)
	}
	return nil
}

func assertPolicyType(policy Object, policyType string) error {
	if !sliceContainsString(asSlice(dig(policy, "spec", "policyTypes")), policyType) {
		return failure("policyTypes does not include %s", policyType)
	}
	return nil
}

func assertPolicyHasPort(policy Object, port int) error {
	for _, ingressRule := range asSlice(dig(policy, "spec", "ingress")) {
		for _, portObject := range asSlice(dig(ingressRule, "ports")) {
			if stringValue(dig(portObject, "port")) == fmt.Sprint(port) {
				return nil
			}
		}
	}
	return failure("policy does not allow TCP port %d", port)
}

func policyAllowsNamespaceLabel(policy Object, key string, value string) bool {
	for _, ingressRule := range asSlice(dig(policy, "spec", "ingress")) {
		for _, from := range asSlice(dig(ingressRule, "from")) {
			if stringValue(dig(from, "namespaceSelector", "matchLabels", key)) == value {
				return true
			}
		}
	}
	return false
}

func policyAllowsPodComponent(policy Object, component string) bool {
	for _, ingressRule := range asSlice(dig(policy, "spec", "ingress")) {
		for _, from := range asSlice(dig(ingressRule, "from")) {
			if stringValue(dig(from, "podSelector", "matchLabels", "app.kubernetes.io/component")) == component {
				return true
			}
		}
	}
	return false
}

func assertNoDockerSocket(deployment Object) error {
	if containsDockerSocket(asSlice(dig(deployment, "spec", "template", "spec", "volumes"))) {
		return CheckFailure("browser-worker Deployment mounts a docker.sock volume")
	}
	for _, container := range deploymentContainers(deployment) {
		if containsDockerSocket(asSlice(dig(container, "volumeMounts"))) {
			return CheckFailure("browser-worker container mounts docker.sock")
		}
	}
	return nil
}

func containsDockerSocket(values []any) bool {
	for _, value := range values {
		object := asObject(value)
		for _, candidate := range []string{
			stringValue(object["name"]),
			stringValue(object["mountPath"]),
			stringValue(dig(object, "hostPath", "path")),
		} {
			if strings.Contains(candidate, "docker.sock") || strings.Contains(candidate, "/var/run/docker") {
				return true
			}
		}
	}
	return false
}

func assertContainerRestricted(container Object) error {
	if stringValue(dig(container, "securityContext", "allowPrivilegeEscalation")) != "false" {
		return CheckFailure("allowPrivilegeEscalation must be false")
	}
	if !sliceContainsString(asSlice(dig(container, "securityContext", "capabilities", "drop")), "ALL") {
		return CheckFailure("container capabilities must drop ALL")
	}
	return nil
}

func assertRuntimePodSpec(pod Object) error {
	spec := asObject(pod["spec"])
	if stringValue(spec["restartPolicy"]) != "Never" {
		return CheckFailure("restartPolicy must be Never")
	}
	if stringValue(spec["automountServiceAccountToken"]) != "false" {
		return CheckFailure("automountServiceAccountToken must be false")
	}
	if stringValue(spec["hostNetwork"]) == "true" || stringValue(spec["hostPID"]) == "true" || stringValue(spec["hostIPC"]) == "true" {
		return CheckFailure("host namespaces must be disabled")
	}
	if len(asSlice(spec["volumes"])) > 0 {
		return CheckFailure("runtime Pods must not mount volumes")
	}
	serviceAccount := stringValue(spec["serviceAccountName"])
	if serviceAccount != "" && serviceAccount != "default" {
		return failure("runtime Pod uses unexpected ServiceAccount %q", serviceAccount)
	}
	if err := assertRuntimePodSecurityContext(spec); err != nil {
		return err
	}
	containers := asObjectSlice(spec["containers"])
	if len(containers) != 1 {
		return failure("runtime Pod must have one container, got %d", len(containers))
	}
	container := containers[0]
	if stringValue(container["name"]) != "browser-runtime" {
		return failure("runtime container name is %q", stringValue(container["name"]))
	}
	if err := assertRuntimeContainerSpec(container); err != nil {
		return err
	}
	return nil
}

func assertRuntimePodSecurityContext(spec Object) error {
	if stringValue(dig(spec, "securityContext", "runAsNonRoot")) != "true" {
		return CheckFailure("Pod securityContext.runAsNonRoot must be true")
	}
	if stringValue(dig(spec, "securityContext", "runAsUser")) != "1000" {
		return CheckFailure("Pod securityContext.runAsUser must be 1000")
	}
	if stringValue(dig(spec, "securityContext", "runAsGroup")) != "1000" {
		return CheckFailure("Pod securityContext.runAsGroup must be 1000")
	}
	if stringValue(dig(spec, "securityContext", "seccompProfile", "type")) != "RuntimeDefault" {
		return CheckFailure("Pod seccomp profile must be RuntimeDefault")
	}
	return nil
}

func assertRuntimeContainerSpec(container Object) error {
	if unresolvedImage(stringValue(container["image"])) {
		return failure("runtime container image is unresolved: %s", stringValue(container["image"]))
	}
	if err := assertContainerRestricted(container); err != nil {
		return err
	}
	if stringValue(dig(container, "securityContext", "runAsNonRoot")) != "true" {
		return CheckFailure("container securityContext.runAsNonRoot must be true")
	}
	if stringValue(dig(container, "securityContext", "runAsUser")) != "1000" {
		return CheckFailure("container securityContext.runAsUser must be 1000")
	}
	if stringValue(dig(container, "securityContext", "runAsGroup")) != "1000" {
		return CheckFailure("container securityContext.runAsGroup must be 1000")
	}
	if stringValue(dig(container, "securityContext", "seccompProfile", "type")) != "RuntimeDefault" {
		return CheckFailure("container seccomp profile must be RuntimeDefault")
	}
	if err := assertContainerResources(container); err != nil {
		return err
	}
	for _, port := range []int{9222, 6080} {
		if !containerHasPort(container, port) {
			return failure("runtime container is missing port %d", port)
		}
	}
	return nil
}

func assertContainerResources(container Object) error {
	for _, path := range [][]string{
		{"resources", "requests", "cpu"},
		{"resources", "requests", "memory"},
		{"resources", "limits", "cpu"},
		{"resources", "limits", "memory"},
	} {
		if strings.TrimSpace(stringValue(dig(container, path...))) == "" {
			return failure("container missing resource %s", strings.Join(path, "."))
		}
	}
	return nil
}

func containerHasPort(container Object, port int) bool {
	for _, portObject := range asSlice(container["ports"]) {
		if stringValue(dig(portObject, "containerPort")) == fmt.Sprint(port) {
			return true
		}
	}
	return false
}

func namedContainer(deployment Object, name string) (Object, bool) {
	for _, container := range deploymentContainers(deployment) {
		if stringValue(container["name"]) == name {
			return container, true
		}
	}
	return nil, false
}

func deploymentContainers(deployment Object) []Object {
	return asObjectSlice(dig(deployment, "spec", "template", "spec", "containers"))
}

func containerEnv(container Object) map[string]string {
	result := map[string]string{}
	for _, envVar := range asSlice(container["env"]) {
		object := asObject(envVar)
		result[stringValue(object["name"])] = stringValue(object["value"])
	}
	return result
}

func validationExpressions(policy Object) []string {
	validations := asSlice(dig(policy, "spec", "validations"))
	expressions := make([]string, 0, len(validations))
	for _, validation := range validations {
		expressions = append(expressions, stringValue(dig(validation, "expression")))
	}
	return expressions
}

func missingExpressionSnippets(expressions []string, snippets []string) []string {
	missing := make([]string, 0)
	for _, snippet := range snippets {
		found := false
		for _, expression := range expressions {
			if strings.Contains(expression, snippet) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, snippet)
		}
	}
	return missing
}

func missingNamedResources(resources []Object, names []string) []string {
	byName := resourcesByName(resources)
	missing := make([]string, 0)
	for _, name := range names {
		if _, ok := byName[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func resourcesByName(resources []Object) map[string]Object {
	result := make(map[string]Object, len(resources))
	for _, resource := range resources {
		name := stringValue(dig(resource, "metadata", "name"))
		if name != "" {
			result[name] = resource
		}
	}
	return result
}

func sliceContainsString(values []any, target string) bool {
	for _, value := range values {
		if stringValue(value) == target {
			return true
		}
	}
	return false
}
