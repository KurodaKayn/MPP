package checks

import (
	"fmt"
	"net/http"
	"strings"
)

func (suite *Suite) configuration() {
	suite.reporter.Section("Configuration")
	suite.check("mpp-app-config keys", true, func() (string, error) {
		configMap, err := suite.kubectl.Resource("configmap", "mpp-app-config", suite.config.AppNamespace)
		if err != nil {
			return "", err
		}
		data := asObject(configMap["data"])
		missing := make([]string, 0)
		for _, key := range requiredConfigKeys {
			if _, ok := data[key]; !ok {
				missing = append(missing, key)
			}
		}
		if err := assert(len(missing) == 0, "missing keys: "+strings.Join(missing, ", ")); err != nil {
			return "", err
		}
		unresolved := make([]string, 0)
		for key, value := range data {
			if placeholderValue(value) {
				unresolved = append(unresolved, key)
			}
		}
		if err := assert(len(unresolved) == 0, "unresolved config values: "+strings.Join(unresolved, ", ")); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d keys present", len(requiredConfigKeys)), nil
	})

	suite.check("mpp-app-secrets keys", true, func() (string, error) {
		secret, err := suite.kubectl.Resource("secret", "mpp-app-secrets", suite.config.AppNamespace)
		if err != nil {
			return "", err
		}
		data := asObject(secret["data"])
		missing := make([]string, 0)
		for _, key := range requiredSecretKeys {
			if strings.TrimSpace(stringValue(data[key])) == "" {
				missing = append(missing, key)
			}
		}
		if err := assert(len(missing) == 0, "missing or empty keys: "+strings.Join(missing, ", ")); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d required keys present", len(requiredSecretKeys)), nil
	})

	suite.check("publish-worker dependency config", true, func() (string, error) {
		configMap, err := suite.kubectl.Resource("configmap", "mpp-app-config", suite.config.AppNamespace)
		if err != nil {
			return "", err
		}
		data := asObject(configMap["data"])
		required := map[string]string{
			"DB_HOST":               stringValue(data["DB_HOST"]),
			"REDIS_ADDR":            stringValue(data["REDIS_ADDR"]),
			"CONTENT_PIPELINE_HOST": stringValue(data["CONTENT_PIPELINE_HOST"]),
			"CONTENT_PIPELINE_PORT": stringValue(data["CONTENT_PIPELINE_PORT"]),
		}
		keys := []string{"DB_HOST", "REDIS_ADDR", "CONTENT_PIPELINE_HOST", "CONTENT_PIPELINE_PORT"}
		empty := make([]string, 0)
		for _, key := range keys {
			if strings.TrimSpace(required[key]) == "" {
				empty = append(empty, key)
			}
		}
		if err := assert(len(empty) == 0, "publish dependencies are empty: "+strings.Join(empty, ", ")); err != nil {
			return "", err
		}
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+required[key])
		}
		return strings.Join(parts, ", "), nil
	})
}

func (suite *Suite) internalReadiness() {
	suite.reporter.Section("Internal Readiness")
	suite.inClusterHTTP("frontend readiness", "http://frontend:3000/api/ready")
	suite.inClusterHTTP("backend readiness", "http://backend:8080/ready")
	suite.inClusterHTTP("ai-service readiness", "http://ai-service:8000/ready")
	suite.inClusterHTTP("collab-service readiness", "http://collab-service:8090/ready")
	suite.inClusterHTTP("content-pipeline metrics", "http://content-pipeline-service:9090/metrics")

	suite.check("browser-worker readiness from backend", true, func() (string, error) {
		body, err := suite.kubectl.Exec(
			"deployment/backend",
			[]string{"wget", "-qO-", fmt.Sprintf("--timeout=%d", suite.config.RequestTimeout), "http://browser-worker:8081/ready"},
			suite.config.AppNamespace,
			"backend",
		)
		if err != nil {
			return "", err
		}
		if err := assert(strings.Contains(body, "ready"), "browser-worker readiness response did not contain ready"); err != nil {
			return "", err
		}
		return "backend can reach browser-worker", nil
	})

	suite.check("publish-worker readiness in its Pod", true, func() (string, error) {
		body, err := suite.kubectl.Exec(
			"deployment/publish-worker",
			[]string{"wget", "-qO-", fmt.Sprintf("--timeout=%d", suite.config.RequestTimeout), "http://127.0.0.1:8080/ready"},
			suite.config.AppNamespace,
			"publish-worker",
		)
		if err != nil {
			return "", err
		}
		if err := assert(strings.Contains(body, "ready"), "publish-worker readiness response did not contain ready"); err != nil {
			return "", err
		}
		return "publish-worker dependencies are ready", nil
	})
}

func (suite *Suite) inClusterHTTP(name string, targetURL string) {
	suite.check(name, true, func() (string, error) {
		body, err := suite.kubectl.CurlFromEphemeralPod(
			suite.config.AppNamespace,
			suite.config.CurlImage,
			targetURL,
			suite.config.RequestTimeout,
			nil,
			http.MethodGet,
			"",
		)
		if err != nil {
			return "", err
		}
		if err := assert(strings.TrimSpace(body) != "", targetURL+" returned an empty body"); err != nil {
			return "", err
		}
		return targetURL + " responded", nil
	})
}

func placeholderValue(value any) bool {
	text := stringValue(value)
	return strings.Contains(text, "replace-with-") ||
		strings.Contains(text, "replace-me") ||
		strings.HasPrefix(text, "registry.example.invalid/")
}
