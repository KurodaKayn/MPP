package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Object map[string]any

type CommandError struct {
	Command  []string
	Stdout   string
	Stderr   string
	ExitCode int
}

func (err *CommandError) Error() string {
	return "kubectl command failed"
}

func (err *CommandError) Message() string {
	parts := []string{fmt.Sprintf("%s exited %d", strings.Join(err.Command, " "), err.ExitCode)}
	if strings.TrimSpace(err.Stderr) != "" {
		parts = append(parts, "stderr: "+strings.TrimSpace(err.Stderr))
	}
	if strings.TrimSpace(err.Stdout) != "" {
		parts = append(parts, "stdout: "+strings.TrimSpace(err.Stdout))
	}
	return strings.Join(parts, "; ")
}

type RunOptions struct {
	Input        string
	AllowFailure bool
}

type Kubectl struct {
	reporter *Reporter
	dryRun   bool
}

func NewKubectl(reporter *Reporter, dryRun bool) *Kubectl {
	return &Kubectl{reporter: reporter, dryRun: dryRun}
}

func (kubectl *Kubectl) Run(args []string, options RunOptions) (string, error) {
	command := append([]string{"kubectl"}, args...)
	kubectl.reporter.Command(command, kubectl.dryRun)
	if kubectl.dryRun {
		return kubectl.dryRunStdout(command), nil
	}

	cmd := exec.Command(command[0], command[1:]...)
	if options.Input != "" {
		cmd.Stdin = strings.NewReader(options.Input)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && !options.AllowFailure {
		exitCode := -1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
		return stdout.String(), &CommandError{
			Command:  command,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}
	}
	return stdout.String(), nil
}

func (kubectl *Kubectl) JSON(args ...string) (Object, error) {
	raw, err := kubectl.Run(append(args, "-o", "json"), RunOptions{})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return Object{}, nil
	}
	var result Object
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, CheckFailure("kubectl returned invalid JSON: " + err.Error())
	}
	return result, nil
}

func (kubectl *Kubectl) CurrentContext() (string, error) {
	output, err := kubectl.Run([]string{"config", "current-context"}, RunOptions{})
	return strings.TrimSpace(output), err
}

func (kubectl *Kubectl) ClientVersion() (any, error) {
	version, err := kubectl.JSON("version", "--client")
	if err == nil {
		return version, nil
	}
	var commandError *CommandError
	if !as(err, &commandError) {
		return nil, err
	}
	output, fallbackErr := kubectl.Run([]string{"version", "--client"}, RunOptions{})
	return strings.TrimSpace(output), fallbackErr
}

func (kubectl *Kubectl) Namespace(name string) (Object, error) {
	return kubectl.JSON("get", "namespace", name)
}

func (kubectl *Kubectl) Resource(kind string, name string, namespace string) (Object, error) {
	args := []string{"get", kind, name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	return kubectl.JSON(args...)
}

func (kubectl *Kubectl) ResourceList(kind string, namespace string, selector string) ([]Object, error) {
	args := []string{"get", kind}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	if selector != "" {
		args = append(args, "-l", selector)
	}
	object, err := kubectl.JSON(args...)
	if err != nil {
		return nil, err
	}
	return asObjectSlice(object["items"]), nil
}

func (kubectl *Kubectl) RolloutStatus(resource string, namespace string, timeout int) (string, error) {
	output, err := kubectl.Run(
		[]string{"rollout", "status", resource, "-n", namespace, fmt.Sprintf("--timeout=%ds", timeout)},
		RunOptions{},
	)
	return strings.TrimSpace(output), err
}

func (kubectl *Kubectl) AuthCanI(verb string, resource string, asUser string, namespace string) (string, error) {
	output, err := kubectl.Run(
		[]string{"auth", "can-i", verb, resource, "--as=" + asUser, "-n", namespace},
		RunOptions{},
	)
	return strings.TrimSpace(output), err
}

func (kubectl *Kubectl) Exec(resource string, command []string, namespace string, container string) (string, error) {
	args := []string{"exec", resource, "-n", namespace}
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, "--")
	args = append(args, command...)
	return kubectl.Run(args, RunOptions{})
}

func (kubectl *Kubectl) CurlFromEphemeralPod(namespace string, image string, targetURL string, timeout int, headers map[string]string, method string, body string) (string, error) {
	pod := "mpp-smoke-curl-" + randomHex(4)
	if !kubectl.dryRun {
		defer kubectl.DeletePod(namespace, pod)
	}

	args := []string{
		"run",
		pod,
		"-n",
		namespace,
		"--image",
		image,
		"--restart=Never",
		"--attach",
		"--rm",
		"--quiet",
		"--labels",
		"app.kubernetes.io/name=mpp,app.kubernetes.io/component=smoke-test",
		"--command",
		"--",
		"curl",
		"-fsS",
		"--max-time",
		fmt.Sprint(timeout),
		"-X",
		method,
	}
	for key, value := range headers {
		args = append(args, "-H", fmt.Sprintf("%s: %s", key, value))
	}
	if body != "" {
		args = append(args, "-H", "Content-Type: application/json", "--data", body)
	}
	args = append(args, targetURL)
	return kubectl.Run(args, RunOptions{})
}

func (kubectl *Kubectl) DeletePod(namespace string, pod string) {
	_, _ = kubectl.Run(
		[]string{"delete", "pod", pod, "-n", namespace, "--ignore-not-found=true", "--wait=false"},
		RunOptions{AllowFailure: true},
	)
}

func (kubectl *Kubectl) dryRunStdout(command []string) string {
	args := command[1:]
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "config":
		return "dry-run-context\n"
	case "version":
		return jsonResponse(Object{"clientVersion": Object{"gitVersion": "dry-run"}})
	case "get":
		return kubectl.dryRunGet(args)
	case "rollout":
		return "dry-run rollout ok\n"
	case "auth":
		return "yes\n"
	case "exec", "run":
		return `{"status":"ready"}`
	default:
		return ""
	}
}

func (kubectl *Kubectl) dryRunGet(args []string) string {
	kind := ""
	name := ""
	if len(args) > 1 {
		kind = args[1]
	}
	if len(args) > 2 && !strings.HasPrefix(args[2], "-") {
		name = args[2]
	}
	namespace := optionValue(args, "-n")
	selector := optionValue(args, "-l")

	switch kind {
	case "namespace", "namespaces":
		return jsonResponse(dryRunNamespace(name))
	case "serviceaccount", "serviceaccounts":
		return jsonResponse(Object{"metadata": Object{"name": name, "labels": Object{}}})
	case "deployments", "deployment":
		if name != "" {
			return jsonResponse(kubectl.dryRunDeployment(name))
		}
		return jsonResponse(Object{"items": kubectl.dryRunDeployments()})
	case "pods", "pod":
		return jsonResponse(Object{"items": kubectl.dryRunPods(selector)})
	case "endpoints", "endpoint":
		return jsonResponse(Object{
			"metadata": Object{"name": name},
			"subsets":  []Object{{"addresses": []Object{{"ip": "10.0.0.10"}}}},
		})
	case "configmap", "configmaps":
		return jsonResponse(Object{"metadata": Object{"name": name}, "data": dryRunConfigMap()})
	case "secret", "secrets":
		return jsonResponse(Object{"metadata": Object{"name": name}, "data": dryRunSecret()})
	case "networkpolicy", "networkpolicies":
		return jsonResponse(Object{"items": dryRunNetworkPolicies(namespace)})
	case "ingress", "ingresses":
		return jsonResponse(dryRunIngress(name))
	case "validatingadmissionpolicy", "validatingadmissionpolicies":
		return jsonResponse(dryRunAdmissionPolicy(name))
	case "validatingadmissionpolicybinding", "validatingadmissionpolicybindings":
		return jsonResponse(dryRunAdmissionPolicyBinding(name))
	default:
		return jsonResponse(Object{})
	}
}

func (kubectl *Kubectl) dryRunDeployments() []Object {
	deployments := make([]Object, 0, len(defaultDeployments))
	for _, deployment := range defaultDeployments {
		deployments = append(deployments, kubectl.dryRunDeployment(deployment))
	}
	return deployments
}

func (kubectl *Kubectl) dryRunDeployment(name string) Object {
	container := Object{
		"name":  name,
		"image": "ghcr.io/kurodakayn/mpp-" + name + ":sha-dryrun",
		"securityContext": Object{
			"allowPrivilegeEscalation": false,
			"capabilities":             Object{"drop": []any{"ALL"}},
		},
	}
	spec := Object{
		"containers": []Object{container},
	}
	if name == "browser-worker" {
		spec["serviceAccountName"] = "browser-worker-runtime-manager"
		container["env"] = []Object{
			{"name": "BROWSER_RUNTIME_DRIVER", "value": "kubernetes"},
			{"name": "BROWSER_RUNTIME_KUBERNETES_NAMESPACE", "value": "mpp-browser-runtime"},
			{"name": "BROWSER_RUNTIME_IMAGE", "value": "ghcr.io/kurodakayn/mpp-browser-runtime:sha-dryrun"},
			{"name": "BROWSER_RUNTIME_KUBERNETES_CPU_REQUEST", "value": "500m"},
			{"name": "BROWSER_RUNTIME_KUBERNETES_CPU_LIMIT", "value": "1"},
			{"name": "BROWSER_RUNTIME_KUBERNETES_MEMORY_REQUEST", "value": "512Mi"},
			{"name": "BROWSER_RUNTIME_KUBERNETES_MEMORY_LIMIT", "value": "1Gi"},
		}
	}
	return Object{
		"metadata": Object{"name": name},
		"spec":     Object{"template": Object{"spec": spec}},
	}
}

func (kubectl *Kubectl) dryRunPods(selector string) []Object {
	if strings.Contains(selector, "app.kubernetes.io/component=browser-runtime") {
		return []Object{
			{
				"metadata": Object{
					"name": "mpp-browser-session-dry-run",
					"labels": Object{
						"mpp.kurodakayn.dev/runtime-driver": "kubernetes",
						"mpp.kurodakayn.dev/session-id":     "dry-run-session",
						"mpp.kurodakayn.dev/owner-hash":     "dry-run-owner",
					},
					"annotations": Object{"mpp.kurodakayn.dev/expires-at": "2099-01-01T00:00:00Z"},
				},
				"spec": Object{
					"automountServiceAccountToken": false,
					"activeDeadlineSeconds":        900,
					"restartPolicy":                "Never",
					"securityContext": Object{
						"runAsNonRoot": true,
						"runAsUser":    1000,
						"runAsGroup":   1000,
						"seccompProfile": Object{
							"type": "RuntimeDefault",
						},
					},
					"containers": []Object{
						{
							"name":  "browser-runtime",
							"image": "ghcr.io/kurodakayn/mpp-browser-runtime:sha-dryrun",
							"ports": []Object{
								{"name": "cdp", "containerPort": 9222},
								{"name": "stream", "containerPort": 6080},
							},
							"resources": Object{
								"requests": Object{"cpu": "500m", "memory": "512Mi"},
								"limits":   Object{"cpu": "1", "memory": "1Gi"},
							},
							"securityContext": Object{
								"runAsNonRoot":             true,
								"runAsUser":                1000,
								"runAsGroup":               1000,
								"allowPrivilegeEscalation": false,
								"capabilities":             Object{"drop": []any{"ALL"}},
								"seccompProfile":           Object{"type": "RuntimeDefault"},
							},
						},
					},
				},
				"status": Object{"phase": "Running"},
			},
		}
	}
	return []Object{
		{
			"metadata": Object{"name": "mpp-app-dry-run"},
			"status": Object{
				"phase":      "Running",
				"conditions": []Object{{"type": "Ready", "status": "True"}},
			},
		},
	}
}

func dryRunNamespace(name string) Object {
	labels := Object{}
	switch name {
	case "mpp-system":
		labels["mpp.kurodakayn.dev/browser-worker-namespace"] = "true"
		labels["pod-security.kubernetes.io/enforce"] = "restricted"
	case "mpp-browser-runtime":
		labels["mpp.kurodakayn.dev/browser-runtime-namespace"] = "true"
		labels["pod-security.kubernetes.io/enforce"] = "restricted"
	}
	return Object{"metadata": Object{"name": name, "labels": labels}}
}

func dryRunConfigMap() Object {
	return Object{
		"BACKEND_API_BASE_URL":      "http://backend:8080",
		"BROWSER_WORKER_URL":        "http://browser-worker:8081",
		"AI_SERVICE_URL":            "http://ai-service:8000",
		"CONTENT_PIPELINE_HOST":     "content-pipeline-service",
		"CONTENT_PIPELINE_PORT":     "50051",
		"COLLAB_INTERNAL_URL":       "http://collab-service:8090",
		"COLLAB_WEBSOCKET_URL_BASE": "wss://mpp.example.com",
		"DB_HOST":                   "postgres.example.com",
		"DB_SSLMODE":                "verify-full",
		"REDIS_ADDR":                "redis.example.com:6379",
		"REDIS_TLS":                 "true",
	}
}

func dryRunNetworkPolicies(namespace string) []Object {
	if namespace == "mpp-browser-runtime" {
		return []Object{
			{"metadata": Object{"name": "browser-runtime-default-deny"}},
			{
				"metadata": Object{"name": "browser-runtime-private-access"},
				"spec": Object{
					"podSelector": Object{"matchLabels": Object{
						"app.kubernetes.io/component":       "browser-runtime",
						"app.kubernetes.io/name":            "mpp",
						"mpp.kurodakayn.dev/runtime-driver": "kubernetes",
					}},
					"policyTypes": []any{"Ingress", "Egress"},
					"ingress": []Object{
						{
							"from": []Object{
								{
									"namespaceSelector": Object{"matchLabels": Object{"mpp.kurodakayn.dev/browser-worker-namespace": "true"}},
									"podSelector":       Object{"matchLabels": Object{"app.kubernetes.io/component": "browser-worker"}},
								},
							},
							"ports": []Object{{"port": 9222}, {"port": 6080}},
						},
					},
				},
			},
		}
	}
	return []Object{
		defaultDenyNetworkPolicy("mpp-system-default-deny"),
		publicNetworkPolicy("public-frontend-access", "frontend", 3000),
		publicNetworkPolicy("public-collab-access", "collab-service", 8090),
		internalNetworkPolicy("frontend-backend-access", "backend", 8080, "frontend", "content-pipeline-service"),
		internalNetworkPolicy("browser-worker-internal-access", "browser-worker", 8081, "backend", "publish-worker"),
		internalNetworkPolicy("ai-service-internal-access", "ai-service", 8000, "backend", "publish-worker"),
		internalNetworkPolicy("content-pipeline-internal-access", "content-pipeline-service", 50051, "backend", "publish-worker"),
		internalNetworkPolicy("collab-service-internal-access", "collab-service", 8090, "backend", "publish-worker"),
	}
}

func defaultDenyNetworkPolicy(name string) Object {
	return Object{
		"metadata": Object{"name": name},
		"spec": Object{
			"podSelector": Object{},
			"policyTypes": []any{"Ingress"},
		},
	}
}

func publicNetworkPolicy(name string, component string, port int) Object {
	return Object{
		"metadata": Object{"name": name},
		"spec": Object{
			"podSelector": Object{"matchLabels": Object{"app.kubernetes.io/component": component}},
			"policyTypes": []any{"Ingress"},
			"ingress": []Object{
				{
					"from": []Object{
						{"namespaceSelector": Object{"matchLabels": Object{"mpp.kurodakayn.dev/public-ingress": "true"}}},
					},
					"ports": []Object{{"port": port}},
				},
			},
		},
	}
}

func internalNetworkPolicy(name string, component string, port int, callers ...string) Object {
	from := make([]Object, 0, len(callers))
	for _, caller := range callers {
		from = append(from, Object{"podSelector": Object{"matchLabels": Object{"app.kubernetes.io/component": caller}}})
	}
	return Object{
		"metadata": Object{"name": name},
		"spec": Object{
			"podSelector": Object{"matchLabels": Object{"app.kubernetes.io/component": component}},
			"policyTypes": []any{"Ingress"},
			"ingress": []Object{
				{
					"from":  from,
					"ports": []Object{{"port": port}},
				},
			},
		},
	}
}

func dryRunIngress(name string) Object {
	return Object{
		"metadata": Object{"name": name},
		"spec": Object{
			"ingressClassName": "nginx",
			"tls": []Object{
				{"hosts": []any{"mpp.example.com"}, "secretName": "mpp-public-tls"},
			},
			"rules": []Object{
				{
					"host": "mpp.example.com",
					"http": Object{"paths": []Object{
						{"path": "/collab", "pathType": "Prefix", "backend": Object{"service": Object{"name": "collab-service"}}},
						{"path": "/", "pathType": "Prefix", "backend": Object{"service": Object{"name": "frontend"}}},
					}},
				},
			},
		},
	}
}

func dryRunAdmissionPolicy(name string) Object {
	return Object{
		"metadata": Object{"name": name},
		"spec": Object{
			"failurePolicy": "Fail",
			"validations": []Object{
				{"expression": "object.metadata.name.startsWith('mpp-browser-')"},
				{"expression": "object.spec.restartPolicy == 'Never'"},
				{"expression": "object.spec.automountServiceAccountToken == false"},
				{"expression": "object.spec.containers.size() == 1"},
				{"expression": "object.spec.containers[0].ports.exists(port, port.containerPort == 9222) && object.spec.containers[0].ports.exists(port, port.containerPort == 6080)"},
				{"expression": "object.spec.containers.all(c, has(c.resources.requests) && has(c.resources.limits))"},
				{"expression": "object.spec.containers.all(c, has(c.securityContext.allowPrivilegeEscalation) && c.securityContext.allowPrivilegeEscalation == false)"},
			},
		},
	}
}

func dryRunAdmissionPolicyBinding(name string) Object {
	return Object{
		"metadata": Object{"name": name},
		"spec": Object{
			"policyName":        name,
			"validationActions": []any{"Deny"},
			"matchResources": Object{"namespaceSelector": Object{"matchLabels": Object{
				"mpp.kurodakayn.dev/browser-runtime-namespace": "true",
			}}},
		},
	}
}

func dryRunSecret() Object {
	secret := make(Object, len(requiredSecretKeys))
	for _, key := range requiredSecretKeys {
		secret[key] = "encoded-value"
	}
	return secret
}

func optionValue(args []string, option string) string {
	for index, arg := range args {
		if arg == option && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func jsonResponse(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw) + "\n"
}

func randomHex(byteCount int) string {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(value)
}
