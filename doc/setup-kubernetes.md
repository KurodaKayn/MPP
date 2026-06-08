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
deploy/kubernetes/overlays/staging-managed
deploy/kubernetes/overlays/staging-self-hosted
deploy/kubernetes/overlays/production-managed
deploy/kubernetes/validation/app-baseline
```

Use `browser-runtime-control` with every Kubernetes browser-runtime deployment.
Use `app-baseline` for the long-running application services. Add
`observability` when the cluster has Loki, Alloy, and Prometheus Operator CRDs.
Choose exactly one data-service mode: `managed` for production, or
`self-hosted` for small test clusters and demos.
Use `overlays/staging-managed` as a renderable starter when staging should use
managed PostgreSQL and Redis endpoints, or `overlays/staging-self-hosted` when
staging should run PostgreSQL and Redis inside the cluster.
Use `overlays/production-managed` as the production starter for managed
PostgreSQL and Redis deployments that materialize `mpp-app-secrets` through an
external secret manager or controlled bootstrap workflow.

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

Label the namespace that runs the public Ingress controller before applying the
app baseline:

```bash
kubectl label namespace <ingress-controller-namespace> \
  mpp.kurodakayn.dev/public-ingress=true --overwrite
```

The app namespace denies ingress to MPP Pods by default. The baseline allows
only the public Ingress namespace to reach `frontend` and `collab-service`, and
allows expected service-to-service traffic from `frontend`, `backend`, and
`publish-worker`. The optional observability package separately opens metrics
scrape ports to namespaces labeled `mpp.kurodakayn.dev/metrics-scraper=true`.
Use that label only on trusted Prometheus namespaces. NetworkPolicy is a layer
4 control, so shared HTTP listener services expose the full service port to
that namespace, not only the `/metrics` path; content-pipeline uses a dedicated
metrics listener on `9090`.

The included `deploy/kubernetes/overlays/staging-managed` and
`deploy/kubernetes/overlays/staging-self-hosted` overlays wire the baseline app,
browser runtime controls, and one data-service mode together. They still contain
example image tags, public host values, provider host values, and generated
Secret literals, so patch those inputs through your environment workflow before
applying either overlay to a shared cluster.
The included `deploy/kubernetes/overlays/production-managed` overlay wires the
same baseline to managed PostgreSQL and Redis without rendering
`mpp-app-secrets`; create that Secret through the production secret workflow
before applying app workloads, then replace the checked-in example hosts and
image tags.

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

For local staging or a one-time bootstrap, generate the app/internal random
values, add provider-supplied values, and render a Secret manifest to a
temporary file:

```bash
secrets_env="$(mktemp)"
script/secret/gen_app_secrets.py app > "$secrets_env"
script/secret/gen_app_secrets.py db >> "$secrets_env"
printf 'LLM_PROVIDER_KEY=%s\n' "$LLM_PROVIDER_KEY" >> "$secrets_env"

ruby script/kubernetes/render-app-secret.rb \
  --env-file "$secrets_env" \
  > /tmp/mpp-app-secrets.yaml
kubectl apply -f /tmp/mpp-app-secrets.yaml
```

When Redis auth is enabled, add `REDIS_PASSWORD` and require it during render:

```bash
script/secret/gen_app_secrets.py redis >> "$secrets_env"
ruby script/kubernetes/render-app-secret.rb \
  --env-file "$secrets_env" \
  --require-redis-password \
  > /tmp/mpp-app-secrets.yaml
```

The renderer emits Kubernetes `stringData`, rejects placeholder-looking values
by default, and ignores env keys that do not belong in `mpp-app-secrets`.

## Database And Redis

Production deployments should use managed PostgreSQL and Redis.

For managed PostgreSQL, keep `DB_SSLMODE=verify-full` and set `DB_HOST` to the
provider hostname, not the `postgres` in-cluster alias, so certificate hostname
verification succeeds. If the provider requires a custom CA bundle, mount it
into app Pods and set `DB_SSLROOTCERT` to the mounted file path. For self-hosted
PostgreSQL, either configure TLS on the StatefulSet or patch
`DB_SSLMODE=disable`.

For managed Redis, set `REDIS_TLS=true` when the provider requires TLS and set
`REDIS_ADDR` to the provider hostname and port, not the `redis` in-cluster
alias, so Redis TLS certificate hostname verification succeeds. Keep the
managed Redis ExternalName Service patched to the same provider hostname for
tooling and non-verifying discovery paths.

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
metrics-scraper namespace and allows that namespace to scrape app metrics. If
Prometheus runs elsewhere, add `mpp.kurodakayn.dev/metrics-scraper=true` to its
namespace only when that namespace is trusted to reach the selected app ports.
The NetworkPolicies are L4 port allowlists, so backend, publish-worker,
browser-worker, AI, and collaboration metrics reuse their shared HTTP listeners;
only content-pipeline exposes a dedicated metrics listener. Install the
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
  ruby script/kubernetes/validate-rendered-schema.rb "$dir" "$rendered"
done
```

The repository Ruby validator rejects unresolved app images, `latest` tags,
missing validation overlay secrets, app workload security-context regressions,
missing probes or resource requests, broken Service and Ingress wiring, browser
runtime RBAC or NetworkPolicy drift, missing observability rules, and malformed
managed or self-hosted data-service packages.

The schema validator wraps `kubeconform` in strict mode against Kubernetes
1.33 schemas. It ignores missing schemas so Prometheus Operator CRDs such as
`PodMonitor` and `PrometheusRule` can still be included while built-in
Kubernetes resources receive API schema coverage. CI installs kubeconform
`v0.8.0`; local validation can use the same binary or set `KUBECONFORM_BIN`.

When validating a staging overlay after replacing the checked-in example values,
enable deployable validation to reject `.example.invalid` hosts, all-zero SHA
image tags, and generated example Secret values:

```bash
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-self-hosted \
  /tmp/mpp-staging.yaml
```

For `deploy/kubernetes/overlays/production-managed`, deployable validation
rejects `.example.invalid` hosts, all-zero SHA image tags, and example model
values. The overlay does not render `mpp-app-secrets`, so secret value
validation belongs to the external secret manager, sealed-secret, or bootstrap
workflow that creates the Secret.

For the final environment overlay, also run an admission dry-run against the
target cluster to catch cluster-specific admission policies, enabled API
versions, and CRD schemas that are outside the repository validation scope.

## Deploy

Apply the environment overlay:

```bash
kubectl apply -k deploy/kubernetes/overlays/<environment>
kubectl rollout status deployment/frontend -n mpp-system
kubectl rollout status deployment/backend -n mpp-system
kubectl rollout status deployment/browser-worker -n mpp-system
kubectl rollout status deployment/collab-service -n mpp-system
```

For the included managed staging starter:

```bash
kubectl kustomize deploy/kubernetes/overlays/staging-managed > /tmp/mpp-staging.yaml
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-managed \
  /tmp/mpp-staging.yaml
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
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url https://mpp.example.com
)
```

The smoke harness checks Deployment rollout status, Service endpoints,
application ConfigMap and Secret shape, internal readiness paths, publish-worker
dependencies, browser runtime RBAC, and runtime Pod cleanup metadata.

Add authenticated user-flow probes when a disposable smoke user token is
available:

```bash
export MPP_SMOKE_AUTH_TOKEN=<bearer-token>
export MPP_SMOKE_PROJECT_ID=<existing-project-id>
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url https://mpp.example.com \
    --run-user-flow-probes
)
```

Use the browser session probe only in environments where creating and cancelling
a remote browser runtime session is acceptable:

```bash
export MPP_SMOKE_AUTH_TOKEN=<bearer-token>
(
  cd script/kubernetes/smoke-test
  go run . \
    --public-url https://mpp.example.com \
    --run-browser-session-probe
)
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
