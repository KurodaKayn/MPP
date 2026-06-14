package checks

import "strings"

func DryRunNamespace(name string) Object {
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

func DryRunDeployments() []Object {
	deployments := make([]Object, 0, len(defaultDeployments))
	for _, deployment := range defaultDeployments {
		deployments = append(deployments, DryRunDeployment(deployment))
	}
	return deployments
}

func DryRunDeployment(name string) Object {
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

func DryRunPods(selector string) []Object {
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

func DryRunConfigMap() Object {
	return Object{
		"BACKEND_API_BASE_URL":                         "http://backend:8080",
		"BROWSER_WORKER_URL":                           "http://browser-worker:8081",
		"AI_SERVICE_URL":                               "http://ai-service:8000",
		"CONTENT_PIPELINE_HOST":                        "content-pipeline-service",
		"CONTENT_PIPELINE_PORT":                        "50051",
		"CONTENT_PIPELINE_MEDIA_RESOLVER_URL":          "http://backend:8080/internal/media/resolve",
		"CONTENT_PIPELINE_MEDIA_OBJECT_STORE":          "r2",
		"CONTENT_PIPELINE_MEDIA_OBJECT_PREFIX":         "content-pipeline/processed-media",
		"CONTENT_PIPELINE_MEDIA_OBJECT_REF_PREFIX":     "mpp://content-pipeline/media/",
		"CONTENT_PIPELINE_MEDIA_OBJECT_RETENTION_DAYS": "7",
		"COLLAB_INTERNAL_URL":                          "http://collab-service:8090",
		"COLLAB_WEBSOCKET_URL_BASE":                    "wss://mpp.example.com",
		"DB_HOST":                                      "postgres.example.com",
		"DB_SSLMODE":                                   "verify-full",
		"REDIS_ADDR":                                   "redis.example.com:6379",
		"REDIS_TLS":                                    "true",
		"OBJECT_STORAGE_PROVIDER":                      "r2",
		"R2_ACCOUNT_ID":                                "dry-run-r2-account",
		"R2_BUCKET":                                    "dry-run-r2-bucket",
		"R2_ENDPOINT":                                  "https://dry-run-r2-account.r2.cloudflarestorage.com",
		"R2_REGION":                                    "auto",
		"X_OAUTH2_CLIENT_ID":                           "dry-run-x-oauth2-client-id",
		"X_OAUTH2_REDIRECT_URL":                        "https://mpp.example.com/api/user/dashboard/settings/x/oauth2/callback",
		"X_OAUTH2_AUTHORIZE_URL":                       "",
		"X_OAUTH2_TOKEN_URL":                           "",
	}
}

func DryRunNetworkPolicies(namespace string) []Object {
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

func DryRunIngress(name string) Object {
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

func DryRunAdmissionPolicy(name string) Object {
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

func DryRunAdmissionPolicyBinding(name string) Object {
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

func DryRunSecret() Object {
	secret := make(Object, len(requiredSecretKeys))
	for _, key := range requiredSecretKeys {
		secret[key] = "encoded-value"
	}
	return secret
}
