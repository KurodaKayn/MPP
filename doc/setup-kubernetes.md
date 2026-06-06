# Kubernetes Production Setup

This guide covers the Kubernetes deployment path for MPP. Docker Compose remains
the local development and production-style smoke-test path; Kubernetes is the
production orchestration path for teams that need rolling deploys, scheduling,
runtime isolation, and managed data services.

## Prerequisites

- A Kubernetes cluster with NetworkPolicy support.
- An Ingress controller. The baseline targets `ingressClassName: nginx`.
- A metrics pipeline if you enable the included HPAs.
- Registry-published images for every MPP service and the browser runtime.
- PostgreSQL and Redis, preferably managed services for production.
- A TLS certificate Secret for the public Ingress.

## Packages

The deployment resources are Kustomize packages:

```text
deploy/kubernetes/browser-runtime-control
deploy/kubernetes/app-baseline
deploy/kubernetes/observability
deploy/kubernetes/data-services/managed
deploy/kubernetes/data-services/self-hosted
deploy/kubernetes/overlays/staging-self-hosted
deploy/kubernetes/validation/app-baseline
```

Use `browser-runtime-control` with every Kubernetes browser-runtime deployment.
Use `app-baseline` for the long-running application services. Add
`observability` when the cluster has Loki, Alloy, and Prometheus Operator CRDs.
Choose exactly one data-service mode: `managed` for production, or
`self-hosted` for small test clusters and demos.
Use `overlays/staging-self-hosted` as a renderable starter when staging should
run PostgreSQL and Redis inside the cluster.

## Required Overlays

Create an environment overlay that references:

- `../../browser-runtime-control`
- `../../app-baseline`
- `../../observability`, if Kubernetes log and metrics discovery is enabled
- `../../data-services/managed` or `../../data-services/self-hosted`

The overlay must patch:

- Every `registry.example.invalid/...:replace-me` image to a registry image with
  an immutable tag.
- `BROWSER_RUNTIME_IMAGE` to the browser runtime image tag.
- `mpp-public-gateway` host, TLS hosts, TLS Secret, and ingress class.
- `DB_HOST`, `REDIS_ADDR`, `COLLAB_WEBSOCKET_URL_BASE`, `LLM_PROVIDER_URL`, and
  `LLM_MODEL`.
- Data-service hosts for the managed ExternalName Services, or storage classes
  and sizes for self-hosted StatefulSets.
- `LOKI_WRITE_URL` in the observability package when Loki is not available at
  the included in-cluster service DNS name.

The included `deploy/kubernetes/overlays/staging-self-hosted` overlay wires the
baseline app, browser runtime controls, and self-hosted data services together.
It still contains example image tags, public host values, and generated Secret
literals, so patch those inputs through your environment workflow before
applying it to a shared cluster.

## Images

The `Container Images` GitHub Actions workflow publishes production images to
GitHub Container Registry on pushes to `main`, release tags matching `v*`, and
manual dispatches. Pull requests that change service source trees or the image
workflow build every service and browser runtime image without pushing, so
Dockerfile and image-context breakages are caught before merge.

Published images use this naming scheme:

```text
ghcr.io/kurodakayn/mpp-frontend
ghcr.io/kurodakayn/mpp-backend
ghcr.io/kurodakayn/mpp-browser-worker
ghcr.io/kurodakayn/mpp-browser-runtime
ghcr.io/kurodakayn/mpp-ai-service
ghcr.io/kurodakayn/mpp-content-pipeline-service
ghcr.io/kurodakayn/mpp-collab-service
```

Every image receives an immutable `sha-<full-git-sha>` tag. Pushes to `main`
also receive a `main` tag, and release tag pushes receive the matching release
tag. Production overlays should pin the `sha-*` tags for app images and set
`BROWSER_RUNTIME_IMAGE` to the matching browser runtime image tag. The
`mpp-backend` image contains both the backend API and publish-worker binaries;
the Deployment command selects the runtime role.

Set the repository variables `FRONTEND_BASE_URL` and `BACKEND_API_BASE_URL`
before publishing images when the frontend build should use values other than
the validation defaults. `BACKEND_API_BASE_URL` is used by Next.js rewrites at
build time, so it must point at the backend URL that the published frontend
image should proxy to.

## Secrets

Create `mpp-app-secrets` in `mpp-system` with at least:

```text
JWT_SECRET
DB_PASSWORD
COLLAB_TOKEN_SECRET
COOKIE_ENCRYPTION_KEY
LLM_PROVIDER_KEY
AI_SERVICE_INTERNAL_TOKEN
BROWSER_WORKER_INTERNAL_TOKEN
CONTENT_PIPELINE_INTERNAL_TOKEN
```

Add `REDIS_PASSWORD` when Redis auth is enabled. Use an external secret manager
or sealed-secret workflow for production; do not commit real Secret values.

## Database And Redis

Production deployments should use managed PostgreSQL and Redis.

For managed PostgreSQL, keep `DB_SSLMODE=verify-full` and set `DB_HOST` to the
provider hostname, not the `postgres` in-cluster alias, so certificate hostname
verification succeeds. If the provider requires a custom CA bundle, mount it
into app Pods and set `DB_SSLROOTCERT` to the mounted file path. For self-hosted
PostgreSQL, either configure TLS on the StatefulSet or patch
`DB_SSLMODE=disable`.

For managed Redis, set `REDIS_TLS=true` when the provider requires TLS.

Schema migration remains a backend startup responsibility. Do not run database
migrations as Kubernetes manifest side effects.

## Browser Runtime

Set:

```text
BROWSER_RUNTIME_DRIVER=kubernetes
BROWSER_RUNTIME_KUBERNETES_NAMESPACE=mpp-browser-runtime
BROWSER_RUNTIME_IMAGE=<registry>/mpp-browser-runtime:<immutable-tag>
```

`browser-worker` creates one restricted runtime Pod per browser session. Runtime
Pods carry session labels, an expiration annotation, an active deadline, and are
reconciled by the worker cleanup loop. The runtime namespace denies traffic by
default and allows CDP/stream ingress only from `browser-worker`.

## Observability

The optional `deploy/kubernetes/observability` package replaces Docker log
discovery with Kubernetes Pod discovery. It deploys Alloy with RBAC scoped to
Pod and Pod log discovery for `mpp-system` and `mpp-browser-runtime`, sends logs
to `LOKI_WRITE_URL`, and preserves structured request fields such as trace ID,
route, status, and latency when services emit JSON request logs.

The package also adds PodMonitor resources for application metrics and
PrometheusRule alerts for browser runtime startup failures, cleanup failures,
cleanup lag, service readiness failures, Redis-dependent readiness failures,
and publish-worker job failures. It labels `mpp-observability` as a
metrics-scraper namespace and allows that namespace to scrape browser-worker
metrics; if Prometheus runs elsewhere, add
`mpp.kurodakayn.dev/metrics-scraper=true` to its namespace. Install the
Prometheus Operator CRDs before applying this package, or omit it from overlays
that use another metrics discovery mechanism.

## Validate

Render and validate every package before applying:

```bash
find deploy/kubernetes -name kustomization.yaml -print | sort | while IFS= read -r package; do
  dir="$(dirname "$package")"
  rendered="$(mktemp)"
  kubectl kustomize "$dir" > "$rendered"
  ruby script/kubernetes/validate-rendered-manifests.rb "$dir" "$rendered"
done
```

The repository Ruby validator rejects unresolved app images, `latest` tags,
missing validation overlay secrets, app workload security-context regressions,
missing probes or resource requests, broken Service and Ingress wiring, browser
runtime RBAC or NetworkPolicy drift, missing observability rules, and malformed
managed or self-hosted data-service packages.

When validating a staging overlay after replacing the checked-in example values,
enable deployable validation to reject `.example.invalid` hosts, all-zero SHA
image tags, and generated example Secret values:

```bash
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-self-hosted \
  /tmp/mpp-staging.yaml
```

For the final environment overlay, also run your cluster's schema validator or
admission dry-run. Tools such as kubeconform or kubeval can complement the
repository policy checks with Kubernetes API schema coverage.

## Deploy

Apply the environment overlay:

```bash
kubectl apply -k deploy/kubernetes/overlays/<environment>
kubectl rollout status deployment/frontend -n mpp-system
kubectl rollout status deployment/backend -n mpp-system
kubectl rollout status deployment/browser-worker -n mpp-system
kubectl rollout status deployment/collab-service -n mpp-system
```

For the included self-hosted staging starter:

```bash
kubectl kustomize deploy/kubernetes/overlays/staging-self-hosted > /tmp/mpp-staging.yaml
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-self-hosted \
  /tmp/mpp-staging.yaml
```

After replacing the example host, image tags, and Secret material, rerun the
same command with `MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1` before applying it to a
shared cluster.

Smoke test:

```bash
ruby script/kubernetes/smoke-test.rb \
  --public-url https://mpp.example.com
```

The smoke harness checks Deployment rollout status, Service endpoints,
application ConfigMap and Secret shape, internal readiness paths, publish-worker
dependencies, browser runtime RBAC, and runtime Pod cleanup metadata.

Add authenticated user-flow probes when a disposable smoke user token is
available:

```bash
MPP_SMOKE_AUTH_TOKEN=<bearer-token> \
MPP_SMOKE_PROJECT_ID=<existing-project-id> \
ruby script/kubernetes/smoke-test.rb \
  --public-url https://mpp.example.com \
  --run-user-flow-probes
```

Use the browser session probe only in environments where creating and cancelling
a remote browser runtime session is acceptable:

```bash
MPP_SMOKE_AUTH_TOKEN=<bearer-token> \
ruby script/kubernetes/smoke-test.rb \
  --public-url https://mpp.example.com \
  --run-browser-session-probe
```

## Operations

Use the full Kubernetes operations runbook for incident response, planned
maintenance, and release promotion:

```text
doc/kubernetes-operations-runbook.md
```

Rollback an app deployment:

```bash
kubectl rollout undo deployment/backend -n mpp-system
kubectl rollout status deployment/backend -n mpp-system
```

Clean expired browser runtime Pods:

```bash
kubectl delete pod -n mpp-browser-runtime \
  -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime,mpp.kurodakayn.dev/runtime-driver=kubernetes
```

Scale a service:

```bash
kubectl scale deployment/backend -n mpp-system --replicas=3
```

Rotate app secrets by updating `mpp-app-secrets`, then restart affected
Deployments:

```bash
kubectl rollout restart deployment/backend deployment/publish-worker deployment/collab-service -n mpp-system
```

Backups, restores, retention, and maintenance are provider responsibilities for
managed data services. For self-hosted data services, configure backup tooling
outside these manifests before using the package for anything beyond tests or
small installations.
