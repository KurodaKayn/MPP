package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kurodakayn/mpp-kubernetes-smoke/checks"
)

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
	if !errors.As(err, &commandError) {
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
	return checks.AsObjectSlice(object["items"]), nil
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
		return jsonResponse(checks.DryRunNamespace(name))
	case "serviceaccount", "serviceaccounts":
		return jsonResponse(Object{"metadata": Object{"name": name, "labels": Object{}}})
	case "deployments", "deployment":
		if name != "" {
			return jsonResponse(checks.DryRunDeployment(name))
		}
		return jsonResponse(Object{"items": checks.DryRunDeployments()})
	case "pods", "pod":
		return jsonResponse(Object{"items": checks.DryRunPods(selector)})
	case "endpoints", "endpoint":
		return jsonResponse(Object{
			"metadata": Object{"name": name},
			"subsets":  []Object{{"addresses": []Object{{"ip": "10.0.0.10"}}}},
		})
	case "configmap", "configmaps":
		return jsonResponse(Object{"metadata": Object{"name": name}, "data": checks.DryRunConfigMap()})
	case "secret", "secrets":
		return jsonResponse(Object{"metadata": Object{"name": name}, "data": checks.DryRunSecret()})
	case "networkpolicy", "networkpolicies":
		return jsonResponse(Object{"items": checks.DryRunNetworkPolicies(namespace)})
	case "ingress", "ingresses":
		return jsonResponse(checks.DryRunIngress(name))
	case "validatingadmissionpolicy", "validatingadmissionpolicies":
		return jsonResponse(checks.DryRunAdmissionPolicy(name))
	case "validatingadmissionpolicybinding", "validatingadmissionpolicybindings":
		return jsonResponse(checks.DryRunAdmissionPolicyBinding(name))
	default:
		return jsonResponse(Object{})
	}
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
