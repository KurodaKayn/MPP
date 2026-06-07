package main

import (
	"fmt"
	"strings"
)

const runtimePodSelector = "app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime,mpp.kurodakayn.dev/runtime-driver=kubernetes"

func (suite *Suite) runtimeControls() {
	suite.reporter.Section("Browser Runtime Control")
	suite.check("runtime namespace NetworkPolicies", true, func() (string, error) {
		policies, err := suite.kubectl.ResourceList("networkpolicy", suite.config.RuntimeNamespace, "")
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(policies))
		for _, policy := range policies {
			names = append(names, stringValue(dig(policy, "metadata", "name")))
		}
		required := []string{"browser-runtime-default-deny", "browser-runtime-private-access"}
		missing := make([]string, 0)
		for _, name := range required {
			if !contains(names, name) {
				missing = append(missing, name)
			}
		}
		if err := assert(len(missing) == 0, "missing NetworkPolicies: "+strings.Join(missing, ", ")); err != nil {
			return "", err
		}
		return "policies=" + strings.Join(required, ","), nil
	})

	for _, verb := range []string{"create", "get", "list", "watch", "delete"} {
		verb := verb
		suite.check("runtime manager can "+verb+" Pods", true, func() (string, error) {
			answer, err := suite.kubectl.AuthCanI(verb, "pods", suite.runtimeManagerServiceAccount(), suite.config.RuntimeNamespace)
			if err != nil {
				return "", err
			}
			if err := assertEqual("yes", answer, "expected kubectl auth can-i to return yes"); err != nil {
				return "", err
			}
			return "allowed", nil
		})
	}
}

func (suite *Suite) runtimeCleanupState() {
	suite.reporter.Section("Browser Runtime Cleanup")
	suite.check("runtime Pod cleanup state", true, func() (string, error) {
		pods, err := suite.kubectl.ResourceList("pods", suite.config.RuntimeNamespace, runtimePodSelector)
		if err != nil {
			return "", err
		}
		if len(pods) == 0 {
			return "no active runtime Pods", nil
		}
		stale := make([]string, 0)
		for _, pod := range pods {
			if staleRuntimePod(pod) {
				stale = append(stale, stringValue(dig(pod, "metadata", "name")))
			}
		}
		if err := assert(len(stale) == 0, "stale runtime Pods: "+strings.Join(stale, ", ")); err != nil {
			return "", err
		}
		missingMetadata := make([]string, 0)
		for _, pod := range pods {
			if runtimeMetadataMissing(pod) {
				missingMetadata = append(missingMetadata, stringValue(dig(pod, "metadata", "name")))
			}
		}
		if err := assert(len(missingMetadata) == 0, "runtime Pods missing session metadata: "+strings.Join(missingMetadata, ", ")); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d active runtime Pods have cleanup metadata", len(pods)), nil
	})
}

func (suite *Suite) runtimeManagerServiceAccount() string {
	return "system:serviceaccount:" + suite.config.AppNamespace + ":browser-worker-runtime-manager"
}

func staleRuntimePod(pod Object) bool {
	phase := stringValue(dig(pod, "status", "phase"))
	return phase == "Succeeded" || phase == "Failed" || dig(pod, "metadata", "deletionTimestamp") != nil
}

func runtimeMetadataMissing(pod Object) bool {
	labels := asObject(dig(pod, "metadata", "labels"))
	annotations := asObject(dig(pod, "metadata", "annotations"))
	return stringValue(labels["mpp.kurodakayn.dev/runtime-driver"]) != "kubernetes" ||
		stringValue(labels["mpp.kurodakayn.dev/session-id"]) == "" ||
		stringValue(labels["mpp.kurodakayn.dev/owner-hash"]) == "" ||
		stringValue(annotations["mpp.kurodakayn.dev/expires-at"]) == ""
}
