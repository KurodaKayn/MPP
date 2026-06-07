package main

import (
	"fmt"
	"strings"
)

func (suite *Suite) preflight() {
	suite.reporter.Section("Preflight")
	suite.check("kubectl context", true, func() (string, error) {
		context, err := suite.kubectl.CurrentContext()
		if err != nil {
			return "", err
		}
		if err := assertPresent(context, "kubectl current context is empty"); err != nil {
			return "", err
		}
		return "context=" + context, nil
	})
	suite.check("kubectl client version", true, func() (string, error) {
		version, err := suite.kubectl.ClientVersion()
		if err != nil {
			return "", err
		}
		return describeClientVersion(version), nil
	})
}

func (suite *Suite) clusterShape() {
	suite.reporter.Section("Cluster Shape")
	for _, namespace := range []string{suite.config.AppNamespace, suite.config.RuntimeNamespace} {
		namespace := namespace
		suite.check("namespace "+namespace, true, func() (string, error) {
			resource, err := suite.kubectl.Namespace(namespace)
			if err != nil {
				return "", err
			}
			if err := assertEqual(namespace, stringValue(dig(resource, "metadata", "name")), "namespace name mismatch"); err != nil {
				return "", err
			}
			labels := asObject(dig(resource, "metadata", "labels"))
			return "labels=" + strings.Join(sortedKeys(labels), ","), nil
		})
	}

	suite.check("runtime manager ServiceAccount", true, func() (string, error) {
		account, err := suite.kubectl.Resource("serviceaccount", "browser-worker-runtime-manager", suite.config.AppNamespace)
		if err != nil {
			return "", err
		}
		if err := assertEqual(
			"browser-worker-runtime-manager",
			stringValue(dig(account, "metadata", "name")),
			"missing browser-worker runtime ServiceAccount",
		); err != nil {
			return "", err
		}
		return "namespace=" + suite.config.AppNamespace, nil
	})
}

func (suite *Suite) workloadRollouts() {
	suite.reporter.Section("Workloads")
	for _, deployment := range defaultDeployments {
		deployment := deployment
		suite.check("rollout deployment/"+deployment, true, func() (string, error) {
			return suite.kubectl.RolloutStatus("deployment/"+deployment, suite.config.AppNamespace, suite.config.RolloutTimeout)
		})
	}

	suite.check("app Pod readiness", true, func() (string, error) {
		pods, err := suite.kubectl.ResourceList("pods", suite.config.AppNamespace, "app.kubernetes.io/name=mpp")
		if err != nil {
			return "", err
		}
		if err := assert(len(pods) > 0, "no app Pods found"); err != nil {
			return "", err
		}
		notReady := make([]string, 0)
		for _, pod := range pods {
			if !podReady(pod) {
				notReady = append(notReady, stringValue(dig(pod, "metadata", "name")))
			}
		}
		if err := assert(len(notReady) == 0, "not-ready Pods: "+strings.Join(notReady, ", ")); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d Pods ready", len(pods)), nil
	})

	suite.check("immutable app images", true, func() (string, error) {
		deployments, err := suite.kubectl.ResourceList("deployments", suite.config.AppNamespace, "")
		if err != nil {
			return "", err
		}
		images := make([]string, 0)
		for _, deployment := range deployments {
			images = append(images, deploymentImages(deployment)...)
		}
		if err := assert(len(images) > 0, "no container images found"); err != nil {
			return "", err
		}
		unresolved := make([]string, 0)
		for _, image := range images {
			if unresolvedImage(image) {
				unresolved = append(unresolved, image)
			}
		}
		if err := assert(len(unresolved) == 0, "unresolved images: "+strings.Join(unresolved, ", ")); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d images checked", len(images)), nil
	})
}

func (suite *Suite) serviceEndpoints() {
	suite.reporter.Section("Service Discovery")
	for _, service := range defaultServices {
		service := service
		suite.check("Service "+service+" endpoints", true, func() (string, error) {
			endpoint, err := suite.kubectl.Resource("endpoints", service, suite.config.AppNamespace)
			if err != nil {
				return "", err
			}
			addresses := endpointAddresses(endpoint)
			if err := assert(len(addresses) > 0, "Service "+service+" has no ready endpoint addresses"); err != nil {
				return "", err
			}
			return fmt.Sprintf("%d endpoint addresses", len(addresses)), nil
		})
	}
}

func describeClientVersion(version any) string {
	if text, ok := version.(string); ok {
		return text
	}
	gitVersion := stringValue(dig(version, "clientVersion", "gitVersion"))
	if gitVersion == "" {
		gitVersion = stringValue(dig(version, "clientVersion", "git_version"))
	}
	if gitVersion != "" {
		return "client=" + gitVersion
	}
	return "client version detected"
}

func podReady(pod Object) bool {
	if dig(pod, "metadata", "deletionTimestamp") != nil {
		return false
	}
	for _, condition := range asSlice(dig(pod, "status", "conditions")) {
		condition := asObject(condition)
		if stringValue(condition["type"]) == "Ready" {
			return stringValue(condition["status"]) == "True"
		}
	}
	return false
}

func deploymentImages(deployment Object) []string {
	containers := asSlice(dig(deployment, "spec", "template", "spec", "containers"))
	images := make([]string, 0, len(containers))
	for _, container := range containers {
		images = append(images, stringValue(asObject(container)["image"]))
	}
	return images
}

func unresolvedImage(image string) bool {
	return image == "" ||
		strings.HasPrefix(image, "mpp-") ||
		strings.HasSuffix(image, ":latest") ||
		strings.Contains(image, "replace-me") ||
		strings.HasPrefix(image, "registry.example.invalid/")
}

func endpointAddresses(endpoint Object) []string {
	subsets := asSlice(endpoint["subsets"])
	addresses := make([]string, 0)
	for _, subset := range subsets {
		for _, address := range asSlice(asObject(subset)["addresses"]) {
			ip := stringValue(asObject(address)["ip"])
			if ip != "" {
				addresses = append(addresses, ip)
			}
		}
	}
	return addresses
}
