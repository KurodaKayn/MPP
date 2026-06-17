# Kubernetes Operations Runbook

This runbook is the day-2 operating guide for MPP on Kubernetes. It assumes the
cluster renders the packages described in `doc/setup-kubernetes.md`, with
application workloads in `mpp-system`, browser runtime Pods in
`mpp-browser-runtime`, and optional observability components in
`mpp-observability`.

Use this document during incidents, planned rollouts, and routine maintenance.
It is written to be executable from a workstation with `kubectl`, `gh`, and
access to the target cluster context.

## Operating Model

- `frontend` serves the public Next.js application and proxies backend API
  requests.
- `backend` serves dashboard, auth, account, project, publishing, and browser
  session APIs.
- `publish-worker` consumes Redis-backed publish jobs and uses the same
  `mpp-backend` image as `backend`.
- `browser-worker` creates isolated browser runtime Pods in
  `mpp-browser-runtime`.
- `ai-service` serves AI editing and calibration endpoints.
- `content-pipeline-service` serves gRPC content pipeline calls and exposes
  metrics separately.
- `collab-service` serves collaborative document traffic behind `/collab`.
- PostgreSQL stores durable state.
- Redis stores transient queues, locks, session state, OAuth state, and
  collaboration sync state.

## Namespaces

| Namespace | Purpose |
| --- | --- |
| `mpp-system` | Application Deployments, Services, ConfigMaps, and app Secret. |
| `mpp-browser-runtime` | Per-session browser runtime Pods created by browser-worker. |
| `mpp-observability` | Alloy, PodMonitor resources, and PrometheusRule resources. |

## Core Workloads

| Workload | Namespace | Kind | Primary Ports |
| --- | --- | --- | --- |
| `frontend` | `mpp-system` | Deployment / Service | `3000/http` |
| `backend` | `mpp-system` | Deployment / Service | `8080/http` |
| `publish-worker` | `mpp-system` | Deployment | `8080/http` |
| `browser-worker` | `mpp-system` | Deployment / Service | `8081/http` |
| `ai-service` | `mpp-system` | Deployment / Service | `8000/http` |
| `content-pipeline-service` | `mpp-system` | Deployment / Service | `50051/grpc`, metrics |
| `collab-service` | `mpp-system` | Deployment / Service | `8090/http` |
| `mpp-alloy` | `mpp-observability` | Deployment | `12345/http` |

## Important Resources

| Resource | Namespace | Notes |
| --- | --- | --- |
| `mpp-app-config` | `mpp-system` | Non-secret application configuration. |
| `mpp-app-secrets` | `mpp-system` | Runtime secrets for app workloads. |
| `mpp-public-gateway` | `mpp-system` | Public Ingress for frontend and `/collab`. |
| `browser-worker-runtime-manager` | `mpp-system` | ServiceAccount used by browser-worker. |
| `mpp-system-default-deny` | `mpp-system` | Default deny ingress NetworkPolicy for app Pods. |
| `public-frontend-access` | `mpp-system` | Allows labeled public Ingress namespaces to reach frontend. |
| `public-collab-access` | `mpp-system` | Allows labeled public Ingress namespaces to reach collaboration traffic. |
| `frontend-backend-access` | `mpp-system` | Allows frontend to reach backend. |
| `browser-runtime-manager` | `mpp-browser-runtime` | Role and RoleBinding for runtime Pods. |
| `browser-runtime-default-deny` | `mpp-browser-runtime` | Default deny NetworkPolicy. |
| `browser-runtime-private-access` | `mpp-browser-runtime` | Allows CDP and stream ingress from browser-worker. |
| `browser-worker-internal-access` | `mpp-system` | Allows backend and publish-worker to reach browser-worker. |
| `ai-service-internal-access` | `mpp-system` | Allows backend and publish-worker to reach AI service. |
| `content-pipeline-internal-access` | `mpp-system` | Allows backend and publish-worker to reach content pipeline gRPC. |
| `collab-service-internal-access` | `mpp-system` | Allows backend and publish-worker to reach collaboration service. |
| `*-observability-metrics` | `mpp-system` | Allows trusted metrics namespaces to reach app metrics ports. Shared HTTP listeners are L4 port allowlists, not `/metrics` path ACLs. |
| `mpp-browser-runtime-alerts` | `mpp-observability` | PrometheusRule group for runtime and service alerts. |

## Alert Inventory

| Alert | First Check | Likely Owner |
| --- | --- | --- |
| `MPPBrowserRuntimeStartupFailures` | Browser runtime Pods and browser-worker logs | Backend / DevOps |
| `MPPBrowserRuntimeCleanupFailures` | browser-worker cleanup logs and RBAC | Backend / DevOps |
| `MPPBrowserRuntimeCleanupLagHigh` | Runtime namespace age and cleanup loop | Backend / DevOps |
| `MPPServiceReadinessFailures` | Readiness endpoints and rollout status | Service owner |
| `MPPRedisDependentServiceReadinessFailures` | Redis connectivity and Redis Secret/config | DevOps |
| `MPPPublishWorkerJobFailures` | publish-worker logs and platform adapters | Backend / Operations |

## Severity Guide

| Severity | User Impact | Response |
| --- | --- | --- |
| SEV1 | Login, dashboard, publishing, or collaboration unavailable for most users. | Page primary owner, freeze releases, start incident log. |
| SEV2 | One major workflow degraded, such as browser sessions or publishing. | Assign owner, mitigate within business hours or sooner if revenue critical. |
| SEV3 | Single service alert, isolated platform failure, or elevated latency. | Triage in normal queue, record findings in tracker. |
| SEV4 | Maintenance task, noisy alert, or documentation gap. | Schedule follow-up. |

## Incident Rules

- Prefer read-only commands until the failing component and blast radius are
  understood.
- Do not delete browser runtime Pods blindly during active user sessions unless
  the incident owner confirms user impact is already worse than session loss.
- Do not restart Redis or PostgreSQL during a publish incident until queue and
  lock state are understood.
- Do not roll forward multiple images at once during mitigation.
- Do not edit live Deployments manually when an environment overlay can express
  the change.
- Record the exact image tags, Git SHAs, and overlay path used during every
  deploy or rollback.

## Shell Setup

Set these variables at the start of a session:

```bash
export MPP_ENV=production
export MPP_APP_NS=mpp-system
export MPP_RUNTIME_NS=mpp-browser-runtime
export MPP_OBS_NS=mpp-observability
export MPP_OVERLAY=deploy/kubernetes/overlays/${MPP_ENV}
export MPP_PUBLIC_HOST=mpp.example.invalid
```

Confirm the active context:

```bash
kubectl config current-context
kubectl cluster-info
```

Stop if the context is not the intended cluster.

## Fast Triage

Run this first for most incidents:

```bash
kubectl get nodes
kubectl get deploy -n "$MPP_APP_NS"
kubectl get pod -n "$MPP_APP_NS" -o wide
kubectl get pod -n "$MPP_RUNTIME_NS" -o wide
kubectl get ingress -n "$MPP_APP_NS"
kubectl get events -n "$MPP_APP_NS" --sort-by=.lastTimestamp | tail -40
```

Check rollout health:

```bash
for deployment in frontend backend publish-worker browser-worker ai-service content-pipeline-service collab-service; do
  kubectl rollout status "deployment/${deployment}" -n "$MPP_APP_NS" --timeout=30s
done
```

Check Services and Endpoints:

```bash
kubectl get svc -n "$MPP_APP_NS"
kubectl get endpoints -n "$MPP_APP_NS"
```

Check selected Pod logs:

```bash
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=100
kubectl logs -n "$MPP_APP_NS" deployment/browser-worker --tail=100
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --tail=100
```

## Readiness Endpoint Checks

Use port-forwarding when the Ingress path is suspect:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/frontend 3000:3000
curl -fsS http://127.0.0.1:3000/api/ready
curl -fsS http://127.0.0.1:3000/api/health
```

Backend:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/backend 8080:8080
curl -fsS http://127.0.0.1:8080/ready
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/metrics | head
```

Browser worker:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/browser-worker 8081:8081
curl -fsS http://127.0.0.1:8081/ready
curl -fsS http://127.0.0.1:8081/health
curl -fsS http://127.0.0.1:8081/metrics | head
```

AI service:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/ai-service 8000:8000
curl -fsS http://127.0.0.1:8000/ready
```

Collab service:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/collab-service 8090:8090
curl -fsS http://127.0.0.1:8090/ready
```

## Deployment Preflight

Before applying an overlay:

```bash
git status --short
git rev-parse HEAD
kubectl config current-context
kubectl get ns "$MPP_APP_NS" "$MPP_RUNTIME_NS"
```

Render and validate manifests:

```bash
rendered="$(mktemp)"
kubectl kustomize "$MPP_OVERLAY" > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb "$MPP_OVERLAY" "$rendered"
```

Inspect rendered images:

```bash
grep -n "image:" "$rendered"
grep -n "BROWSER_RUNTIME_IMAGE" -A2 "$rendered"
```

Reject a rollout if any app image uses:

- `:latest`
- `:replace-me`
- `registry.example.invalid`
- a local image name such as `mpp-backend`

Confirm immutable GHCR tags:

```bash
grep -E "ghcr\.io/.+:sha-[0-9a-f]{40}" "$rendered"
```

## Standard Deploy

Apply the overlay:

```bash
kubectl apply -k "$MPP_OVERLAY"
```

Watch rollouts:

```bash
kubectl rollout status deployment/frontend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/browser-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/ai-service -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/content-pipeline-service -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/collab-service -n "$MPP_APP_NS" --timeout=5m
```

Confirm Pods:

```bash
kubectl get pod -n "$MPP_APP_NS" -o wide
kubectl get hpa -n "$MPP_APP_NS"
kubectl get pdb -n "$MPP_APP_NS"
```

Smoke test:

```bash
curl -fsS "https://${MPP_PUBLIC_HOST}/api/ready"
curl -fsS "https://${MPP_PUBLIC_HOST}/api/health"
```

Then manually verify:

- Open the frontend.
- Sign in.
- Load the dashboard.
- Open a collaborative document.
- Start a browser session.
- Stop the browser session.
- Confirm the runtime Pod disappears.

## Rollback

Use rollout undo when the previous ReplicaSet is the desired image set:

```bash
kubectl rollout undo deployment/frontend -n "$MPP_APP_NS"
kubectl rollout undo deployment/backend -n "$MPP_APP_NS"
kubectl rollout undo deployment/publish-worker -n "$MPP_APP_NS"
kubectl rollout undo deployment/browser-worker -n "$MPP_APP_NS"
kubectl rollout undo deployment/collab-service -n "$MPP_APP_NS"
```

Watch the rollback:

```bash
kubectl rollout status deployment/frontend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/browser-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/collab-service -n "$MPP_APP_NS" --timeout=5m
```

Use an overlay revert when the change included config, Secret references, or
multiple image updates:

```bash
git revert <bad-change-sha>
kubectl apply -k "$MPP_OVERLAY"
```

After rollback:

```bash
kubectl get deploy -n "$MPP_APP_NS" -o wide
kubectl get rs -n "$MPP_APP_NS"
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=100
```

## Pausing A Rollout

If Pods are failing during a rollout:

```bash
kubectl rollout pause deployment/backend -n "$MPP_APP_NS"
kubectl describe deployment/backend -n "$MPP_APP_NS"
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=backend
```

Resume after mitigation:

```bash
kubectl rollout resume deployment/backend -n "$MPP_APP_NS"
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
```

## Readiness Failure Alert

Alert names:

- `MPPServiceReadinessFailures`
- `MPPRedisDependentServiceReadinessFailures`

First checks:

```bash
kubectl get pod -n "$MPP_APP_NS"
kubectl describe pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=backend
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=200
kubectl logs -n "$MPP_APP_NS" deployment/browser-worker --tail=200
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --tail=200
```

Check readiness directly:

```bash
kubectl exec -n "$MPP_APP_NS" deployment/backend -- wget -qO- http://127.0.0.1:8080/ready
kubectl exec -n "$MPP_APP_NS" deployment/browser-worker -- wget -qO- http://127.0.0.1:8081/ready
kubectl exec -n "$MPP_APP_NS" deployment/publish-worker -- wget -qO- http://127.0.0.1:8080/ready
```

Likely causes:

- Redis endpoint unreachable.
- Redis auth or TLS mismatch.
- PostgreSQL endpoint unreachable.
- Secret changed without workload restart.
- Bad app image or incompatible config.
- Node-level networking failure.

Mitigations:

- Restore the last known good ConfigMap and Secret.
- Roll back the affected Deployment.
- Restart Redis-dependent workloads after Redis Secret updates.
- Scale down only if the workload is crash-looping and saturating dependencies.

## Frontend Incident

Symptoms:

- Public page returns 5xx.
- `/api/ready` fails through the frontend.
- Static assets are missing.
- API proxy paths fail while backend is healthy.

Checks:

```bash
kubectl get ingress -n "$MPP_APP_NS" mpp-public-gateway -o yaml
kubectl get svc -n "$MPP_APP_NS" frontend backend
kubectl logs -n "$MPP_APP_NS" deployment/frontend --tail=200
kubectl describe deployment/frontend -n "$MPP_APP_NS"
```

Validate build-time API target:

```bash
kubectl exec -n "$MPP_APP_NS" deployment/frontend -- printenv BACKEND_API_BASE_URL
```

Mitigation:

- Roll back `frontend` if the image is suspect.
- Patch overlay build variables before publishing a new image if rewrites point
  to the wrong backend URL.
- Check Ingress host and TLS Secret if only public traffic is affected.

## Backend Incident

Symptoms:

- Login fails.
- Dashboard APIs fail.
- Publish requests do not enqueue.
- Browser session APIs fail.

Checks:

```bash
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=300
kubectl describe deployment/backend -n "$MPP_APP_NS"
kubectl exec -n "$MPP_APP_NS" deployment/backend -- printenv BACKEND_REQUIRE_REDIS
kubectl exec -n "$MPP_APP_NS" deployment/backend -- printenv DB_HOST DB_SSLMODE REDIS_ADDR
```

Check database and Redis connectivity from a temporary diagnostic Pod if policy
allows it:

```bash
DB_HOST="$(kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o jsonpath='{.data.DB_HOST}')"
REDIS_ADDR="$(kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o jsonpath='{.data.REDIS_ADDR}')"
kubectl run -n "$MPP_APP_NS" netcheck --rm -it --restart=Never \
  --env="DB_HOST=${DB_HOST}" \
  --env="REDIS_ADDR=${REDIS_ADDR}" \
  --image=busybox:1.37 -- sh
```

Inside the Pod:

```sh
nc -vz "$DB_HOST" 5432
nc -vz "$(echo "$REDIS_ADDR" | cut -d: -f1)" "$(echo "$REDIS_ADDR" | cut -d: -f2)"
```

Mitigation:

- Roll back backend image for API regressions.
- Restore DB or Redis configuration for readiness failures.
- Restart `backend` after Secret rotation.
- Do not clear Redis keys unless publish and browser session impact is known.

## Publish Worker Incident

Alert:

- `MPPPublishWorkerJobFailures`

Symptoms:

- Publishing remains pending.
- Publishing fails for one platform.
- Publish jobs retry repeatedly.
- Browser-based publishers fail after runtime startup.

Checks:

```bash
kubectl get deploy -n "$MPP_APP_NS" publish-worker
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --tail=300
kubectl exec -n "$MPP_APP_NS" deployment/publish-worker -- wget -qO- http://127.0.0.1:8080/ready
kubectl exec -n "$MPP_APP_NS" deployment/publish-worker -- wget -qO- http://127.0.0.1:8080/metrics | grep mpp_publish_jobs_total
```

Check Redis-related readiness:

```bash
kubectl exec -n "$MPP_APP_NS" deployment/publish-worker -- printenv REDIS_ADDR REDIS_TLS
```

Check platform-specific failure shape:

```bash
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --since=30m | grep -i "publish job"
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --since=30m | grep -i "failed"
```

Mitigation:

- If all platforms fail, check Redis, DB, and shared backend image.
- If one platform fails, check platform account state, cookie validity, and
  provider-side changes before rolling back.
- If browser-based platforms fail, continue with the browser runtime incident
  section.
- Scale `publish-worker` cautiously; more replicas can increase provider rate
  pressure.

## Browser Runtime Startup Incident

Alert:

- `MPPBrowserRuntimeStartupFailures`

Symptoms:

- Remote login sessions do not open.
- Browser-backed publishing fails before platform automation starts.
- Runtime Pods remain Pending, ImagePullBackOff, or CrashLoopBackOff.

Checks:

```bash
kubectl get pod -n "$MPP_RUNTIME_NS" -o wide
kubectl get events -n "$MPP_RUNTIME_NS" --sort-by=.lastTimestamp | tail -60
kubectl logs -n "$MPP_APP_NS" deployment/browser-worker --tail=300
kubectl describe deployment/browser-worker -n "$MPP_APP_NS"
```

Inspect one runtime Pod:

```bash
runtime_pod="$(kubectl get pod -n "$MPP_RUNTIME_NS" \
  -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime \
  -o jsonpath='{.items[0].metadata.name}')"
kubectl describe pod -n "$MPP_RUNTIME_NS" "$runtime_pod"
kubectl logs -n "$MPP_RUNTIME_NS" "$runtime_pod" --tail=200
```

Check browser-worker runtime config:

```bash
kubectl exec -n "$MPP_APP_NS" deployment/browser-worker -- printenv \
  BROWSER_RUNTIME_DRIVER \
  BROWSER_RUNTIME_KUBERNETES_NAMESPACE \
  BROWSER_RUNTIME_IMAGE
```

Likely causes:

- Runtime image tag missing or private without pull permission.
- Runtime namespace missing admission labels.
- Browser-worker ServiceAccount lacks runtime Pod permissions.
- Runtime Pod blocked by NetworkPolicy or admission policy.
- Cluster lacks CPU or memory for Chromium runtime Pods.

Mitigation:

- Patch `BROWSER_RUNTIME_IMAGE` to the last known good immutable image.
- Restore `browser-worker-runtime-manager` ServiceAccount and RoleBinding.
- Roll back `browser-worker` if Pod creation code regressed.
- Add cluster capacity or reduce runtime concurrency if Pods are Pending.

## Browser Runtime Cleanup Incident

Alerts:

- `MPPBrowserRuntimeCleanupFailures`
- `MPPBrowserRuntimeCleanupLagHigh`

Symptoms:

- Old runtime Pods remain after sessions finish.
- Runtime namespace accumulates many completed or failed Pods.
- Cleanup metrics show lag over five minutes.

Checks:

```bash
kubectl get pod -n "$MPP_RUNTIME_NS" \
  -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime \
  --sort-by=.metadata.creationTimestamp
kubectl logs -n "$MPP_APP_NS" deployment/browser-worker --since=1h | grep -i cleanup
kubectl auth can-i delete pods \
  --as=system:serviceaccount:mpp-system:browser-worker-runtime-manager \
  -n "$MPP_RUNTIME_NS"
```

Manual cleanup for expired runtime Pods:

```bash
kubectl delete pod -n "$MPP_RUNTIME_NS" \
  -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime,mpp.kurodakayn.dev/runtime-driver=kubernetes
```

Use manual cleanup only after confirming no active sessions depend on those
Pods. If the incident owner is unsure, prefer deleting only Pods older than the
session TTL:

```bash
kubectl get pod -n "$MPP_RUNTIME_NS" \
  -l app.kubernetes.io/component=browser-runtime \
  --sort-by=.metadata.creationTimestamp
```

Mitigation:

- Restore delete permission in the runtime Role.
- Restart `browser-worker` if cleanup loop is wedged.
- Roll back `browser-worker` if cleanup started failing after a deploy.
- Add alert notes to the incident log with the oldest Pod age.

## Browser Runtime Network Incident

Symptoms:

- Runtime Pods start successfully but browser-worker cannot connect.
- CDP or noVNC stream fails.
- Runtime logs are healthy but session URL is unreachable.

Checks:

```bash
kubectl get netpol -n "$MPP_RUNTIME_NS"
kubectl describe netpol -n "$MPP_RUNTIME_NS" browser-runtime-private-access
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=browser-worker --show-labels
kubectl get pod -n "$MPP_RUNTIME_NS" --show-labels
```

Verify DNS egress policy:

```bash
kubectl describe netpol -n "$MPP_RUNTIME_NS" browser-runtime-private-access
kubectl describe netpol -n "$MPP_RUNTIME_NS" browser-runtime-default-deny
```

Mitigation:

- Restore browser-worker labels required by NetworkPolicy selectors.
- Restore runtime Pod labels required by NetworkPolicy selectors.
- Check cluster CNI NetworkPolicy support.
- Roll back runtime control manifests if traffic broke after policy changes.

## Collaboration Incident

Symptoms:

- Collaborative document socket fails.
- `/collab` path returns 404 or 502.
- Multiple users cannot join a document.

Checks:

```bash
kubectl get ingress -n "$MPP_APP_NS" mpp-public-gateway -o yaml
kubectl get svc -n "$MPP_APP_NS" collab-service
kubectl logs -n "$MPP_APP_NS" deployment/collab-service --tail=300
kubectl exec -n "$MPP_APP_NS" deployment/collab-service -- printenv COLLAB_WS_PATH COLLAB_REDIS_SYNC_ENABLED REDIS_ADDR
```

Mitigation:

- Roll back `collab-service` if socket handling regressed.
- Restore Ingress `/collab` path routing.
- Check Redis if multi-replica collaboration sync fails.
- Restart `collab-service` after rotating `COLLAB_TOKEN_SECRET`.

## AI Service Incident

Symptoms:

- AI edit requests fail.
- Calibration endpoints return provider errors.
- The frontend works but AI features are degraded.

Checks:

```bash
kubectl logs -n "$MPP_APP_NS" deployment/ai-service --tail=300
kubectl exec -n "$MPP_APP_NS" deployment/ai-service -- printenv LLM_PROVIDER_URL LLM_MODEL
kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets -o jsonpath='{.data.LLM_PROVIDER_KEY}' | wc -c
```

Mitigation:

- Restore `LLM_PROVIDER_URL`, `LLM_MODEL`, or provider key.
- Roll back `ai-service` for application regressions.
- Degrade gracefully by disabling AI-facing UI only if the product owner
  confirms that core publishing remains more important.

## Content Pipeline Incident

Symptoms:

- Draft compilation or media pipeline calls fail.
- gRPC readiness fails.
- Backend logs show content pipeline connection errors.

Checks:

```bash
kubectl get svc -n "$MPP_APP_NS" content-pipeline-service
kubectl logs -n "$MPP_APP_NS" deployment/content-pipeline-service --tail=300
kubectl exec -n "$MPP_APP_NS" deployment/backend -- printenv CONTENT_PIPELINE_HOST CONTENT_PIPELINE_PORT
```

Check metrics if Prometheus is not available:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/content-pipeline-service 9090:9090
curl -fsS http://127.0.0.1:9090/metrics | head
```

Mitigation:

- Roll back `content-pipeline-service`.
- Restore service name and port config if backend cannot reach the gRPC service.

## Redis Incident

Symptoms:

- Redis-dependent readiness alert fires.
- Publish queues stop.
- Browser sessions cannot be created.
- Collaboration sync degrades.

Checks:

```bash
kubectl get svc -n "$MPP_APP_NS" redis
kubectl get endpoints -n "$MPP_APP_NS" redis
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=200 | grep -i redis
kubectl logs -n "$MPP_APP_NS" deployment/browser-worker --tail=200 | grep -i redis
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --tail=200 | grep -i redis
```

Check config:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o yaml | grep -E "REDIS_ADDR|REDIS_TLS|REDIS_DB"
kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets -o jsonpath='{.data.REDIS_PASSWORD}' | wc -c
```

Mitigation:

- Restore Redis endpoint or ExternalName.
- Restore `REDIS_TLS` to match the provider.
- Restore Redis auth Secret.
- Restart `backend`, `publish-worker`, `browser-worker`, and `collab-service`
  after Redis Secret changes.
- For managed Redis, follow the provider failover runbook before restarting MPP
  workloads.

## Redis Persistence Baseline

Persistence policy:

| Environment | Mechanism | Policy | Restart expectation |
| --- | --- | --- | --- |
| `staging-self-hosted` | Redis StatefulSet with `redis-data` PVC mounted at `/data` | AOF enabled with `appendfsync everysec`; RDB snapshots use `save 900 1`, `save 300 10`, and `save 60 10000` | Normal Redis Pod restarts keep persisted keys that have not expired. Writes inside the last AOF fsync window may be lost after node or storage failure. |
| `staging-managed` | Managed Redis provider | Provider persistence or snapshots must be selected in the staging provider configuration | Follow the provider restart and restore guarantees. Do not assume in-cluster PVC recovery. |
| `production-managed` | Managed Redis provider | Provider-backed persistence or snapshots, retention, and restore point objective must be recorded before production use | Follow the provider restart, failover, and restore runbook. |

Self-hosted checks:

```bash
kubectl get configmap -n "$MPP_APP_NS" redis-persistence-config -o yaml
kubectl get statefulset -n "$MPP_APP_NS" redis -o jsonpath='{.spec.volumeClaimTemplates[*].metadata.name}{"\n"}'
kubectl get pvc -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    redis-cli --no-auth-warning -a "$REDIS_PASSWORD" CONFIG GET dir appendonly appendfsync save
  else
    redis-cli CONFIG GET dir appendonly appendfsync save
  fi
'
```

Health and graceful shutdown check:

```bash
kubectl get statefulset -n "$MPP_APP_NS" redis -o jsonpath='{.spec.template.spec.terminationGracePeriodSeconds}{"\n"}'
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    redis-cli --no-auth-warning -a "$REDIS_PASSWORD" ping
  else
    redis-cli ping
  fi
'
```

Controlled non-production readiness failure:

```bash
redis_pod="$(kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis -o jsonpath='{.items[0].metadata.name}')"
kubectl exec -n "$MPP_APP_NS" "$redis_pod" -- sh -ec 'kill -STOP 1'
until [ "$(kubectl get pod -n "$MPP_APP_NS" "$redis_pod" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')" = "False" ]; do
  sleep 5
done
kubectl exec -n "$MPP_APP_NS" "$redis_pod" -- sh -ec 'kill -CONT 1'
kubectl wait -n "$MPP_APP_NS" --for=condition=Ready pod/"$redis_pod" --timeout=120s
```

Run the failure check only in non-production. It verifies that the readiness
probe fails when Redis stops serving commands, and that the Pod becomes ready
again after the process resumes.

Non-production restart check:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    redis-cli --no-auth-warning -a "$REDIS_PASSWORD" SET mpp:persistence:probe "$(date -u +%s)" EX 3600
    redis-cli --no-auth-warning -a "$REDIS_PASSWORD" SET mpp:persistence:ephemeral "$(date -u +%s)" EX 5
  else
    redis-cli SET mpp:persistence:probe "$(date -u +%s)" EX 3600
    redis-cli SET mpp:persistence:ephemeral "$(date -u +%s)" EX 5
  fi
'
kubectl delete pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis
kubectl rollout status statefulset/redis -n "$MPP_APP_NS"
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    redis-cli --no-auth-warning -a "$REDIS_PASSWORD" GET mpp:persistence:probe
    redis-cli --no-auth-warning -a "$REDIS_PASSWORD" GET mpp:persistence:ephemeral
  else
    redis-cli GET mpp:persistence:probe
    redis-cli GET mpp:persistence:ephemeral
  fi
'
```

The probe key should remain after the restart. The short-TTL key may be absent;
that is expected. Do not use this check in production without a maintenance
window because it deletes the Redis Pod.

## Redis Runtime Hardening

The self-hosted Redis runtime policy is versioned in the
`redis-persistence-config` ConfigMap alongside the persistence baseline.

| Setting | Base value | Intent |
| --- | --- | --- |
| `maxmemory` | `384mb` | Keep Redis data memory below the default `512Mi` Pod limit so Redis returns explicit memory errors before Kubernetes kills the process. |
| `maxmemory-policy` | `noeviction` | Protect the current mixed keyspace of locks, user-flow state, queue-like metadata, and caches from silent eviction. Writes fail under memory pressure instead. |
| `timeout` | `0` | Keep pooled and pub/sub clients open; do not disconnect idle app clients at the Redis layer. |
| `tcp-keepalive` | `300` | Let TCP keepalive detect dead peers without aggressive client churn. |
| `appendonly` / `appendfsync` | `yes` / `everysec` | Match the persistence baseline and keep the one-second AOF fsync window explicit. |
| `slowlog-log-slower-than` | `10000` | Record commands slower than 10ms. |
| `slowlog-max-len` | `256` | Retain a bounded recent slow-command history for incident review. |

Auth behavior:

- Self-hosted Redis reads optional `REDIS_PASSWORD` from `mpp-app-secrets`.
- When `REDIS_PASSWORD` is present, the StatefulSet starts Redis with
  `--requirepass`, and probes plus shutdown hooks authenticate with that same
  Secret.
- When `REDIS_PASSWORD` is absent, self-hosted Redis runs without auth for the
  current environment. Managed Redis environments must follow the provider auth
  requirement and materialize `REDIS_PASSWORD` when auth is enabled.

Runtime configuration check:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
    redis-cli --no-auth-warning CONFIG GET \
      maxmemory maxmemory-policy timeout tcp-keepalive \
      appendonly appendfsync slowlog-log-slower-than slowlog-max-len requirepass
    redis-cli --no-auth-warning SLOWLOG LEN
  else
    redis-cli CONFIG GET \
      maxmemory maxmemory-policy timeout tcp-keepalive \
      appendonly appendfsync slowlog-log-slower-than slowlog-max-len requirepass
    redis-cli SLOWLOG LEN
  fi
'
```

Controlled non-production memory pressure rehearsal:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  redis_cli() {
    if [ -n "${REDIS_PASSWORD:-}" ]; then
      redis-cli --no-auth-warning -a "$REDIS_PASSWORD" "$@"
    else
      redis-cli "$@"
    fi
  }

  before_evictions="$(redis_cli INFO stats | awk -F: "/^evicted_keys:/ {print \$2}" | tr -d "\r")"
  redis_cli SET mpp:memory-pressure:baseline ok EX 600
  payload="$(yes x | head -c 1048576)"
  i=0
  until ! redis_cli SET "mpp:memory-pressure:$i" "$payload" EX 600 >/tmp/mpp-redis-pressure.out 2>&1; do
    i=$((i + 1))
  done

  cat /tmp/mpp-redis-pressure.out
  redis_cli GET mpp:memory-pressure:baseline
  after_evictions="$(redis_cli INFO stats | awk -F: "/^evicted_keys:/ {print \$2}" | tr -d "\r")"
  test "$before_evictions" = "$after_evictions"
  redis_cli --scan --pattern "mpp:memory-pressure:*" | while IFS= read -r key; do
    redis_cli DEL "$key" >/dev/null
  done
'
```

Expected rehearsal result:

- The write loop eventually returns a Redis OOM error instead of killing the Pod.
- `mpp:memory-pressure:baseline` remains readable.
- `evicted_keys` does not increase because `maxmemory-policy` is `noeviction`.
- Deleting the rehearsal keys restores write capacity.

Run the memory pressure rehearsal only in non-production or an isolated restore
environment. It intentionally fills Redis up to `maxmemory`.

## Redis Backup And Restore Runbook

The self-hosted Redis backup path is the `redis-backup` CronJob in
`deploy/kubernetes/data-services/self-hosted/backups.yaml`. It writes RDB
snapshots to `/backups/redis` on the `mpp-data-backups` PVC and prunes old
files by `BACKUP_RETENTION_DAYS`.

Operator checklist:

```bash
kubectl get cronjob -n "$MPP_APP_NS" redis-backup
kubectl get job -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis-backup
kubectl logs -n "$MPP_APP_NS" job/redis-backup --tail=200
kubectl get pvc -n "$MPP_APP_NS" mpp-data-backups
```

Run an on-demand backup before maintenance or a restore drill:

```bash
kubectl create job -n "$MPP_APP_NS" --from=cronjob/redis-backup \
  redis-backup-manual-$(date -u +%Y%m%d%H%M%S)
```

Restore procedure for a non-production self-hosted Redis snapshot:

1. Stop all Redis writers. Scale `backend`, `publish-worker`, `browser-worker`,
   and `collab-service` to zero or pause the environment.
2. Copy the chosen `redis-*.rdb` snapshot into a clean Redis data directory as
   `dump.rdb`.
3. Start the replacement pod from a restore-only overlay or temporary config
   that already sets `appendonly no`. Do not rely on `CONFIG SET appendonly no`
   alone; that only changes the live process and is lost on restart.
4. Quarantine or remove any retained AOF files on the mounted PVC, especially
   `/data/appendonlydir`, before the replacement pod starts so Redis cannot
   replay an old AOF over the selected `dump.rdb`.
5. Verify the expected durable keys are present.
6. Re-enable AOF and let Redis rewrite it after the restore is validated.
7. Bring the app workloads back up.

Example pod-level sequence:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
    redis-cli --no-auth-warning SHUTDOWN SAVE || true
  else
    redis-cli SHUTDOWN SAVE || true
  fi
'
# replace /data/dump.rdb from the selected backup snapshot
# quarantine /data/appendonlydir or restore from a clean PVC snapshot before the new pod starts
# deploy the restore-only overlay or temporary StatefulSet patch that sets appendonly no
kubectl delete pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis
kubectl rollout status statefulset/redis -n "$MPP_APP_NS"
kubectl exec -n "$MPP_APP_NS" statefulset/redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
    redis-cli --no-auth-warning GET mpp:persistence:probe
    redis-cli --no-auth-warning CONFIG SET appendonly yes
    redis-cli --no-auth-warning BGREWRITEAOF
  else
    redis-cli GET mpp:persistence:probe
    redis-cli CONFIG SET appendonly yes
    redis-cli BGREWRITEAOF
  fi
'
```

Do not restore into a live writer set. Restore is only safe after Redis writers
are stopped, because in-flight queue, lock, and session state can be lost or
replayed.

Non-production restore drill result:

- 2026-06-16: local `redis:7-alpine` drill using `redis-cli --rdb` backup and
  RDB-first restore.
- Result: restored durable key survived, short-TTL key was absent as expected.
- Restore RTO: `~2s` from Redis start to key verification on the local Docker
  drill.
- RPO: last successful RDB snapshot, plus any writes accepted after that
  snapshot and before restore start.

Recovery expectations:

- Durable session or configuration keys should return from the snapshot.
- Short-TTL keys may expire before or during restore and are not guaranteed.
- Queue-like or lock-like keys can be stale on restore and must be treated as
  operator-recovered state, not a transactional replay guarantee.

## Redis Keyspace Inventory

Use `script/redis/keyspace_inventory.rb` to produce a factual Redis keyspace
inventory for non-production environments. The command is read-only: against
Redis it uses `SCAN`, `TYPE`, `PTTL`, and `MEMORY USAGE`; fixture mode reads a
local JSON sample only. Live Redis scans require `redis-cli` on the operator
machine or inside the environment where the script runs.

Dry-run with the committed fixture:

```bash
ruby script/redis/keyspace_inventory.rb \
  --fixture script/redis/fixtures/keyspace_inventory_sample.json
```

Run against non-production Redis with bounded load:

```bash
REDIS_ADDR=redis.example.invalid:6379 \
REDIS_PASSWORD=... \
REDIS_DB=0 \
ruby script/redis/keyspace_inventory.rb \
  --scan-count 100 \
  --max-keys 10000 \
  --sample-limit 3 \
  > redis-keyspace-inventory.json
```

Keep the default `--match '*'` for the first baseline so non-`mpp:*` families
such as `auth:*` verification keys and `asynq:*` worker queue metadata are not
missed. Use `--match` only for targeted follow-up scans, or run one scan per
family such as `auth:*`, `mpp:*`, and `asynq:*`.

Safety limits:

| Limit | Default | Purpose |
| --- | ---: | --- |
| `--match` | `*` | Restrict scanned keyspace for focused non-production runs. |
| `--scan-count` | `100` | Keeps each incremental SCAN batch small. |
| `--max-keys` | `10000` | Stops the scan before it can walk an unexpectedly large database. |
| `--sample-limit` | `3` | Keeps report size stable while still showing representative keys. |
| `--command-timeout` | `5` seconds | Bounds per-key metadata calls. |

The JSON report is stable for comparison between runs. It contains the observed
key pattern, declared or inferred owner, read/write services where known,
responsibility tier, loss tolerance, recovery expectation, TTL policy plus
observed TTL range, Redis data type counts, memory usage totals, and sample
keys. Unknown patterns are preserved with `owner: "unknown"` and
`responsibility_tier: "unclassified"` so the responsibility baseline can
classify them instead of losing evidence.

## Redis SLO Baseline

The Redis SLO baseline lives in `deploy/kubernetes/data-services/redis-exporter`
and `deploy/kubernetes/observability`. The dashboard covers availability,
p95/p99 command latency, blocked clients, memory usage, memory headroom,
connections, and evictions. Alerts page on Redis unavailability, elevated p99
latency, connection or command errors, low memory headroom, key evictions under
the configured eviction policy, connection count pressure, and blocked clients.

Capacity guardrails:

| Alert | Threshold | Primary signal | Expected operator response |
| --- | --- | --- | --- |
| `MPPRedisMemoryHeadroomLow` | Less than 15% headroom below `maxmemory` for 15 minutes | `redis_memory_max_bytes - redis_memory_used_bytes` | Check top key families with the keyspace inventory, stop runaway writers, delete only documented disposable keys, or raise `maxmemory` and the Pod memory limit together in a reviewed overlay. |
| `MPPRedisUnexpectedKeyEvictions` | Any sustained key eviction while `maxmemory_policy="noeviction"` | `redis_evicted_keys_total` gated by `redis_instance_info{maxmemory_policy="noeviction"}` | Treat as policy drift or provider behavior drift. Confirm `CONFIG GET maxmemory-policy`, inspect recent Redis restarts or provider changes, and preserve lock/session/queue-like keys before deleting data. |
| `MPPRedisConnectionCountHigh` | More than 80% of `maxclients` for 10 minutes | `redis_connected_clients / redis_config_maxclients` | Identify client growth by workload, check app connection-pool settings, restart only leaking clients, and raise `maxclients` only with file descriptor and provider limits checked. |
| `MPPRedisLatencyP99High` | p99 command latency above 50ms for 10 minutes | `redis_latency_percentiles_usec{quantile="0.99"}` | Check slowlog and hot commands, correlate with memory pressure, backup, or traffic bursts, and shed non-critical Redis work before restarting Redis. |

`MPPRedisConnectionCountHigh` depends on Redis exporter access to the
`CONFIG GET maxclients` command. If `redis_config_maxclients` is absent, fix the
exporter ACL or provider configuration before treating the guardrail as
complete.

Non-production simulation record:

- 2026-06-16: `docker run --rm --entrypoint promtool -v <tmp>:/work:ro prom/prometheus:v3.12.0 test rules /work/redis-alerts.test.yml`
- Result: `SUCCESS` for synthetic Redis unavailability, p99 latency, connection
  errors, memory pressure, evictions, and blocked-clients alert cases.
- 2026-06-17: `ruby script/kubernetes/test_redis_capacity_alerts.rb`
- Result: `SUCCESS` for synthetic Redis memory headroom, noeviction-bound key
  eviction, connection count, and p99 command latency alert cases.

### Redis Responsibility Tiers

`script/redis/keyspace_inventory.rb` is the source of declared Redis pattern
metadata. Every declared pattern must record `responsibility_tier`, `owner`,
`ttl_policy`, `loss_tolerance`, and `recovery_expectation`. Treat
`owner_source: "inferred"` or `responsibility_tier: "unclassified"` as a review
blocker for any new Redis family.

| Tier | Label | Use When | Review Expectation |
| --- | --- | --- | --- |
| `R0` | Critical coordination | Redis gates concurrency, lock ownership, idempotency, or mutual exclusion. | Confirm the durable source of truth and how work is prevented from running twice after Redis loss. |
| `R1` | User continuity | Redis holds short-lived user flow state that cannot be recomputed without user action. | Confirm the user-facing retry path and that durable account or project state is not lost. |
| `R2` | Performance cache | Redis stores data that is recomputed from PostgreSQL, object storage, or another durable service. | Confirm TTL, invalidation, and stale-read bounds. |
| `R3` | Ephemeral signal | Redis stores hints such as heartbeat, cleanup, or throttle state. | Confirm stale signals are revalidated before destructive action. |
| `R4` | Queue-like usage | Redis stores queued, scheduled, retained, or worker-owned task state. | Confirm replay, reconciliation, or operator recovery from durable domain records. |

Classification guidance:

- Prefer the highest-impact tier when a pattern serves multiple purposes. For
  example, a cache key that also guards idempotency should be `R0`, not `R2`.
- Assign ownership to the service or library responsible for writes and
  recovery, not merely the service that reads the key.
- Review expected TTLs against observed `PTTL` output. Keys with no Redis TTL
  must still describe how stale members are pruned or regenerated.
- For new Redis usage, add the declared pattern and responsibility metadata
  before merging the feature that writes the key.

## PostgreSQL Incident

Symptoms:

- Backend readiness fails.
- Login and dashboard data fail.
- Publish state cannot be saved.
- Collaboration metadata fails to load.

Checks:

```bash
kubectl get svc -n "$MPP_APP_NS" postgres
kubectl get endpoints -n "$MPP_APP_NS" postgres
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=300 | grep -i "db\\|postgres\\|database"
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o yaml | grep -E "DB_HOST|DB_PORT|DB_SSLMODE|DB_SSLROOTCERT"
```

Check Secret material:

```bash
kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets -o jsonpath='{.data.DB_PASSWORD}' | wc -c
```

Mitigation:

- Restore database endpoint or ExternalName.
- Restore `DB_SSLMODE` and `DB_SSLROOTCERT`.
- Restart backend and publish-worker after DB Secret changes.
- Do not run schema changes from Kubernetes manifests.
- For managed PostgreSQL, use the provider restore/failover runbook.

## Self-Hosted Data Services

The self-hosted package is for small installations and tests. It includes daily
PostgreSQL and Redis backup CronJobs that write to the `mpp-data-backups` PVC.
The PVC is still cluster-local, so copy artifacts to external storage or pair it
with storage-provider snapshots before treating the package as recoverable.

Check Postgres StatefulSet:

```bash
kubectl get statefulset -n "$MPP_APP_NS" postgres
kubectl get pvc -n "$MPP_APP_NS" -l app.kubernetes.io/component=postgres
kubectl logs -n "$MPP_APP_NS" statefulset/postgres --tail=200
```

Check Redis StatefulSet:

```bash
kubectl get statefulset -n "$MPP_APP_NS" redis
kubectl get pvc -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis
kubectl logs -n "$MPP_APP_NS" statefulset/redis --tail=200
```

Check backup schedules and recent runs:

```bash
kubectl get cronjob -n "$MPP_APP_NS" postgres-backup redis-backup
kubectl get job -n "$MPP_APP_NS" -l app.kubernetes.io/component=postgres-backup
kubectl get job -n "$MPP_APP_NS" -l app.kubernetes.io/component=redis-backup
kubectl get pvc -n "$MPP_APP_NS" mpp-data-backups
```

Run an ad hoc backup before maintenance:

```bash
kubectl create job -n "$MPP_APP_NS" --from=cronjob/postgres-backup postgres-backup-manual-$(date -u +%Y%m%d%H%M%S)
kubectl create job -n "$MPP_APP_NS" --from=cronjob/redis-backup redis-backup-manual-$(date -u +%Y%m%d%H%M%S)
```

Mitigation:

- Do not delete PVCs during incidents.
- Scale application workloads down before destructive database maintenance.
- Snapshot the PostgreSQL, Redis, and `mpp-data-backups` PVCs before restore
  attempts.
- Restore PostgreSQL dumps with `pg_restore` into a clean database after
  verifying the dump timestamp and application version.
- Restore Redis RDB snapshots only after stopping Redis writers and confirming
  that losing in-flight queues, sessions, and locks is acceptable.
- Prefer provider-managed services for production.

## Secret Rotation

Prepare:

```bash
kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets -o yaml > /tmp/mpp-app-secrets.before.yaml
```

Apply the new Secret through the environment secret workflow. Do not commit real
Secret values.

Restart affected workloads:

```bash
kubectl rollout restart deployment/backend -n "$MPP_APP_NS"
kubectl rollout restart deployment/publish-worker -n "$MPP_APP_NS"
kubectl rollout restart deployment/browser-worker -n "$MPP_APP_NS"
kubectl rollout restart deployment/collab-service -n "$MPP_APP_NS"
kubectl rollout restart deployment/ai-service -n "$MPP_APP_NS"
```

Watch readiness:

```bash
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/browser-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/collab-service -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/ai-service -n "$MPP_APP_NS" --timeout=5m
```

Secret-to-workload map:

| Secret Key | Workloads |
| --- | --- |
| `JWT_SECRET` | `backend`, `publish-worker` |
| `DB_PASSWORD` | `backend`, `publish-worker` |
| `REDIS_PASSWORD` | `backend`, `publish-worker`, `browser-worker`, `collab-service` |
| `COLLAB_TOKEN_SECRET` | `backend`, `publish-worker`, `collab-service` |
| `COOKIE_ENCRYPTION_KEY` | `backend`, `publish-worker` |
| `LLM_PROVIDER_KEY` | `ai-service` |
| `AI_SERVICE_INTERNAL_TOKEN` | `backend`, `publish-worker`, `ai-service` |
| `BROWSER_WORKER_INTERNAL_TOKEN` | `backend`, `publish-worker`, `browser-worker` |
| `CONTENT_PIPELINE_INTERNAL_TOKEN` | `backend`, `publish-worker`, `content-pipeline-service` |
| `R2_ACCESS_KEY_ID` | `backend`, `publish-worker`, `content-pipeline-service` |
| `R2_SECRET_ACCESS_KEY` | `backend`, `publish-worker`, `content-pipeline-service` |
| `X_OAUTH2_CLIENT_SECRET` | `backend`, `publish-worker` |

## Scaling

Scale a stateless service:

```bash
kubectl scale deployment/backend -n "$MPP_APP_NS" --replicas=3
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
```

Use the HPA where present:

```bash
kubectl get hpa -n "$MPP_APP_NS"
kubectl describe hpa -n "$MPP_APP_NS" backend
```

Guidelines:

- Scale `frontend` and `backend` horizontally for request load.
- Scale `collab-service` only after Redis sync is healthy.
- Scale `publish-worker` carefully because it can increase provider pressure.
- Scale `browser-worker` only when cluster capacity can absorb runtime Pods.
- Do not scale stateful Postgres or Redis from these app runbooks.

## Node Pressure

Symptoms:

- Pods Pending.
- Evictions.
- Image pulls slow or failing.
- Runtime Pods fail under load.

Checks:

```bash
kubectl describe nodes | grep -E "Name:|Pressure|Allocatable|Allocated" -A5
kubectl get events --all-namespaces --sort-by=.lastTimestamp | tail -80
kubectl top nodes
kubectl top pod -n "$MPP_APP_NS"
kubectl top pod -n "$MPP_RUNTIME_NS"
```

Mitigation:

- Reduce browser session concurrency.
- Scale down non-critical workloads.
- Add nodes or increase node size.
- Move runtime Pods to an isolated node pool in the environment overlay if
  supported by the cluster.

## Image Pull Failure

Symptoms:

- `ImagePullBackOff`.
- `ErrImagePull`.
- New rollout stalls before container start.

Checks:

```bash
kubectl get pod -n "$MPP_APP_NS" -o wide
kubectl describe pod -n "$MPP_APP_NS" <pod-name>
kubectl get pod -n "$MPP_RUNTIME_NS" -o wide
kubectl describe pod -n "$MPP_RUNTIME_NS" <runtime-pod-name>
```

Mitigation:

- Confirm the image tag exists in GHCR.
- Confirm image visibility or imagePullSecret setup.
- Patch the overlay back to the previous immutable tag.
- Re-run the container image workflow if the expected SHA tag is missing.

## Ingress And TLS Incident

Symptoms:

- Public URL fails while Services are healthy.
- TLS certificate errors.
- `/collab` path fails but frontend root works.

Checks:

```bash
kubectl get ingress -n "$MPP_APP_NS" mpp-public-gateway -o yaml
kubectl describe ingress -n "$MPP_APP_NS" mpp-public-gateway
kubectl get secret -n "$MPP_APP_NS" mpp-public-tls
kubectl get svc -n "$MPP_APP_NS" frontend collab-service
```

Mitigation:

- Restore host and TLS hosts in the environment overlay.
- Restore `spec.tls[*].secretName`.
- Check ingress controller logs.
- Roll back Ingress changes if only routing changed.

## Observability Incident

Symptoms:

- Logs missing from Loki.
- Pod metrics missing.
- PrometheusRule alerts absent.
- Alloy Pod unhealthy.

Checks:

```bash
kubectl get pod -n "$MPP_OBS_NS"
kubectl logs -n "$MPP_OBS_NS" deployment/mpp-alloy --tail=300
kubectl get podmonitor -n "$MPP_OBS_NS"
kubectl get prometheusrule -n "$MPP_OBS_NS"
kubectl describe deployment -n "$MPP_OBS_NS" mpp-alloy
```

Check Alloy config:

```bash
kubectl get configmap -n "$MPP_OBS_NS" mpp-alloy-config -o yaml
```

Mitigation:

- Restore `LOKI_WRITE_URL`.
- Restore PodMonitor CRDs if missing.
- Restart `mpp-alloy` after config changes.
- Check NetworkPolicy if browser-worker metrics disappear.

## NetworkPolicy Regression

Symptoms:

- Service works through port-forward but not from another Pod.
- Browser-worker cannot reach runtime Pods.
- Prometheus cannot scrape app metrics.

Checks:

```bash
kubectl get netpol -n "$MPP_APP_NS"
kubectl get netpol -n "$MPP_RUNTIME_NS"
kubectl get namespace --show-labels | grep 'mpp.kurodakayn.dev/public-ingress=true'
kubectl get namespace --show-labels | grep 'mpp.kurodakayn.dev/metrics-scraper=true'
kubectl describe netpol -n "$MPP_APP_NS" mpp-system-default-deny
kubectl describe netpol -n "$MPP_APP_NS" public-frontend-access
kubectl describe netpol -n "$MPP_APP_NS" public-collab-access
kubectl describe netpol -n "$MPP_APP_NS" frontend-backend-access
kubectl describe netpol -n "$MPP_APP_NS" browser-worker-internal-access
kubectl describe netpol -n "$MPP_APP_NS" ai-service-internal-access
kubectl describe netpol -n "$MPP_APP_NS" content-pipeline-internal-access
kubectl describe netpol -n "$MPP_APP_NS" collab-service-internal-access
kubectl describe netpol -n "$MPP_APP_NS" backend-worker-observability-metrics
kubectl describe netpol -n "$MPP_APP_NS" browser-worker-observability-metrics
kubectl describe netpol -n "$MPP_APP_NS" ai-service-observability-metrics
kubectl describe netpol -n "$MPP_APP_NS" collab-service-observability-metrics
kubectl describe netpol -n "$MPP_APP_NS" content-pipeline-observability-metrics
kubectl describe netpol -n "$MPP_RUNTIME_NS" browser-runtime-private-access
```

Mitigation:

- Restore labels on source Pods and namespaces.
- Restore `mpp.kurodakayn.dev/public-ingress=true` on the Ingress controller
  namespace.
- Restore `mpp.kurodakayn.dev/metrics-scraper=true` on the Prometheus
  namespace.
- Confirm only trusted Prometheus namespaces carry
  `mpp.kurodakayn.dev/metrics-scraper=true`; shared HTTP listener metrics
  policies allow that namespace to reach the full service port.
- Restore policy selectors from the baseline manifests.
- Roll back policy-only changes first when the app images did not change.

## RBAC Regression

Symptoms:

- browser-worker logs `forbidden` while creating, listing, or deleting runtime
  Pods.
- Cleanup loop fails.
- New sessions fail immediately.

Checks:

```bash
kubectl auth can-i create pods \
  --as=system:serviceaccount:mpp-system:browser-worker-runtime-manager \
  -n "$MPP_RUNTIME_NS"
kubectl auth can-i list pods \
  --as=system:serviceaccount:mpp-system:browser-worker-runtime-manager \
  -n "$MPP_RUNTIME_NS"
kubectl auth can-i delete pods \
  --as=system:serviceaccount:mpp-system:browser-worker-runtime-manager \
  -n "$MPP_RUNTIME_NS"
```

Mitigation:

- Restore the ServiceAccount name on `browser-worker`.
- Restore `browser-runtime-manager` Role and RoleBinding.
- Roll back RBAC changes if permissions narrowed unexpectedly.

## Admission Policy Regression

Symptoms:

- Runtime Pods rejected at create time.
- Events mention `mpp-browser-runtime-pods`.

Checks:

```bash
kubectl get validatingadmissionpolicy mpp-browser-runtime-pods -o yaml
kubectl get validatingadmissionpolicybinding mpp-browser-runtime-pods -o yaml
kubectl get events -n "$MPP_RUNTIME_NS" --sort-by=.lastTimestamp | tail -80
```

Mitigation:

- Confirm runtime Pods carry required labels.
- Confirm runtime Pod names start with `mpp-browser-`.
- Roll back admission policy changes if valid runtime Pods are blocked.

## ConfigMap Change

Before changing `mpp-app-config`:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o yaml > /tmp/mpp-app-config.before.yaml
kubectl diff -k "$MPP_OVERLAY"
```

Apply:

```bash
kubectl apply -k "$MPP_OVERLAY"
```

Restart workloads that read config only at process start:

```bash
kubectl rollout restart deployment/frontend -n "$MPP_APP_NS"
kubectl rollout restart deployment/backend -n "$MPP_APP_NS"
kubectl rollout restart deployment/publish-worker -n "$MPP_APP_NS"
kubectl rollout restart deployment/browser-worker -n "$MPP_APP_NS"
kubectl rollout restart deployment/ai-service -n "$MPP_APP_NS"
kubectl rollout restart deployment/content-pipeline-service -n "$MPP_APP_NS"
kubectl rollout restart deployment/collab-service -n "$MPP_APP_NS"
```

Validate:

```bash
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=100
```

## Release Promotion

Promotion checklist:

- Container Images workflow completed for the target Git SHA.
- For production-managed releases, pin the overlay with
  `ruby script/kubernetes/pin-overlay-images.rb --overlay deploy/kubernetes/overlays/production-managed --git-sha <full-git-sha>`.
- For provider-specific production overlays, run the `Kubernetes Image
  Promotion` workflow with the target overlay, full Git SHA, and image
  namespace, then review and apply the generated `promotion.patch`. The
  workflow verifies every promoted image tag before writing the patch. Enable
  strict deployable validation only when the overlay already uses production
  hosts, secrets, and immutable images.
- All app and browser runtime image tags use `sha-<full-git-sha>`.
- The staging overlay has run successfully with the target images.
- Kubernetes render validation passes.
- Smoke tests passed in staging.
- Rollback tags are known and recorded.
- Database migrations, if any, are backward compatible or have a rollback plan.
- Browser runtime image and browser-worker image are promoted together when the
  runtime contract changes.

Record:

```text
Environment:
Overlay path:
Git SHA:
Frontend image:
Backend image:
Browser-worker image:
Browser runtime image:
AI service image:
Content pipeline image:
Collab service image:
Operator:
Start time:
End time:
Rollback image set:
```

## Post-Deploy Verification

Run:

```bash
kubectl get deploy -n "$MPP_APP_NS"
kubectl get pod -n "$MPP_APP_NS" -o wide
kubectl get pod -n "$MPP_RUNTIME_NS" -o wide
kubectl get events -n "$MPP_APP_NS" --sort-by=.lastTimestamp | tail -30
```

Verify metrics:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/backend 8080:8080
curl -fsS http://127.0.0.1:8080/metrics | grep mpp_http_requests_total | head
```

Manual workflow:

- Login.
- Open dashboard.
- Save project content.
- Sync prepublish drafts.
- Start a browser login session.
- Save cookies for a browser-backed platform if using a test account.
- Trigger a publish to a safe test platform or test account.
- Open a collaborative document in two browser windows.

## Maintenance Windows

Before maintenance:

- Announce expected impact.
- Pause non-critical releases.
- Confirm rollback image set.
- Confirm database backup recency.
- Confirm Redis provider maintenance status.
- Lower publish-worker replicas if provider calls should pause.

During maintenance:

- Keep an incident log.
- Apply one class of change at a time.
- Watch readiness after every restart.
- Stop if unrelated alerts start firing.

After maintenance:

- Restore normal replicas.
- Run post-deploy verification.
- Close the maintenance log with image tags and observed issues.

## Incident Log Template

```text
Incident:
Severity:
Start time:
Detection source:
Primary owner:
Customer impact:
Affected namespace:
Affected workload:
Current image tags:
Recent deploys:
Initial hypothesis:
Mitigation:
Rollback performed:
Resolution time:
Follow-up issues:
```

## Escalation Checklist

Escalate when:

- SEV1 impact lasts more than 15 minutes.
- Data integrity is uncertain.
- PostgreSQL restore or failover is required.
- Redis data loss may affect active queues or sessions.
- Browser runtime Pods cannot be created after RBAC and image checks.
- Multiple independent services fail readiness at the same time.
- Public TLS or DNS is outside the application team's control.

Bring:

- Current `kubectl get deploy -n mpp-system -o wide`.
- Current failing alert names.
- Last successful Git SHA.
- Last applied overlay commit.
- Relevant logs from the failing Deployment.
- Any provider incident links for managed PostgreSQL, Redis, or ingress.

## Postmortem Checklist

Within two business days:

- Write a short timeline.
- Record detection gap and mitigation gap.
- Identify whether the first alert was actionable.
- Identify whether rollback was documented and fast enough.
- Add missing commands or checks to this runbook.
- Add regression tests or CI validation when the failure was preventable.
- Update the Kubernetes deployment plan progress tracker if the incident
  creates new deployment hardening work.

## Command Reference

List app Pods by component:

```bash
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=backend
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=publish-worker
kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/component=browser-worker
```

Describe all unhealthy Pods:

```bash
kubectl get pod -n "$MPP_APP_NS" --field-selector=status.phase!=Running
kubectl get pod -n "$MPP_RUNTIME_NS" --field-selector=status.phase!=Running
```

Tail logs for all Pods of a component:

```bash
kubectl logs -n "$MPP_APP_NS" -l app.kubernetes.io/component=backend --tail=200 --prefix
kubectl logs -n "$MPP_APP_NS" -l app.kubernetes.io/component=browser-worker --tail=200 --prefix
```

Show image tags:

```bash
kubectl get deploy -n "$MPP_APP_NS" \
  -o custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image
```

Show runtime Pod age:

```bash
kubectl get pod -n "$MPP_RUNTIME_NS" \
  -l app.kubernetes.io/component=browser-runtime \
  --sort-by=.metadata.creationTimestamp
```

Run the Kubernetes smoke harness after applying an overlay:

```bash
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url "$MPP_PUBLIC_URL"
)
```

If a required probe fails, the harness automatically prints app/runtime Pod
summaries, recent events, and `browser-worker` Deployment diagnostics. Increase
or reduce per-command output with `--diagnostic-lines`, or pass
`--skip-diagnostics` when diagnostics are captured by CI artifacts.

Use the full staging E2E profile for release verification. This enables public,
authenticated, project-scoped, collaboration, and remote browser-session probes,
and writes JSON/JUnit artifacts:

```bash
export MPP_SMOKE_AUTH_TOKEN=<bearer-token>
export MPP_SMOKE_PROJECT_ID=<existing-project-id>
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url "$MPP_PUBLIC_URL" \
    --full-e2e \
    --report-json smoke-report.json \
    --report-junit smoke-junit.xml
)
```

The `Kubernetes Smoke E2E` GitHub Actions workflow runs this same profile
against a selected protected environment. The environment must provide
`MPP_KUBECONFIG_B64`, `MPP_SMOKE_AUTH_TOKEN`, and `MPP_SMOKE_PROJECT_ID`; the
workflow uploads `smoke.log`, `smoke-report.json`, and `smoke-junit.xml`.

Run authenticated read and project-scoped probes with a disposable user token:

```bash
export MPP_SMOKE_AUTH_TOKEN=<bearer-token>
export MPP_SMOKE_PROJECT_ID=<existing-project-id>
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url "$MPP_PUBLIC_URL" \
    --run-user-flow-probes
)
```

Only run the remote browser session probe when the environment can tolerate a
short-lived runtime Pod:

```bash
export MPP_SMOKE_AUTH_TOKEN=<bearer-token>
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url "$MPP_PUBLIC_URL" \
    --run-browser-session-probe
)
```

Get recent events:

```bash
kubectl get events -n "$MPP_APP_NS" --sort-by=.lastTimestamp | tail -50
kubectl get events -n "$MPP_RUNTIME_NS" --sort-by=.lastTimestamp | tail -50
```

Restart one workload:

```bash
kubectl rollout restart deployment/backend -n "$MPP_APP_NS"
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
```

Render an overlay:

```bash
kubectl kustomize "$MPP_OVERLAY" > /tmp/mpp-rendered.yaml
```

Diff live cluster with overlay:

```bash
kubectl diff -k "$MPP_OVERLAY"
```

Apply an overlay:

```bash
kubectl apply -k "$MPP_OVERLAY"
```

Check runtime manager permissions:

```bash
for verb in create get list watch delete; do
  kubectl auth can-i "$verb" pods \
    --as=system:serviceaccount:mpp-system:browser-worker-runtime-manager \
    -n "$MPP_RUNTIME_NS"
done
```

## Known Non-Goals

- This runbook does not replace cloud provider database failover procedures.
- This runbook does not define a production backup product.
- This runbook does not grant permission to bypass the environment overlay.
- This runbook does not cover Docker Compose operations.
- This runbook does not authorize manual edits that cannot be reconciled back to
  Git.
