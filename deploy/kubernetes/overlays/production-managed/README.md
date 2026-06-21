# Production Managed Overlay

This overlay is the production starter for clusters that run MPP application
workloads in Kubernetes while using managed PostgreSQL and Redis outside the
cluster. It combines:

- `deploy/kubernetes/browser-runtime-control`
- `deploy/kubernetes/app-baseline`
- `deploy/kubernetes/data-services/managed`
- `deploy/kubernetes/external-secrets`

Use this overlay directly only after replacing the checked-in placeholders, or
copy/layer it into a provider-specific production overlay. The production setup
guide documents the AWS, GCP, and Azure patch points for managed PostgreSQL,
managed Redis, External Secrets Operator stores, ingress, TLS, and deployable
validation.

The checked-in values are renderable placeholders only:

- App and browser runtime images use the immutable-looking
  `sha-0000000000000000000000000000000000000000` example tag.
- The public host is `mpp.example.invalid`.
- Managed PostgreSQL and Redis ExternalName targets use `.example.invalid`
  provider hosts.
- `DB_READER_HOST` and the `postgres-reader` ExternalName are placeholders for
  the managed PostgreSQL read replica endpoint.
- `mpp-app-secrets` is generated only at runtime by External Secrets Operator.
  The checked-in `ExternalSecret` references a placeholder
  `mpp-production-secrets` `ClusterSecretStore` and placeholder remote keys.
- Redis persistence is a production provider responsibility in this overlay.
  Production Redis must have provider-backed persistence or snapshots enabled
  before application traffic depends on it.

Before applying this overlay to production:

- Replace every `sha-0000000000000000000000000000000000000000` image tag with
  registry-published `sha-<full-git-sha>` tags:
  `ruby script/kubernetes/pin-overlay-images.rb --overlay deploy/kubernetes/overlays/production-managed --git-sha <full-git-sha>`.
  For non-GHCR provider registries, add
  `--image-namespace <registry>/<namespace>`. The `Kubernetes Image Promotion`
  workflow wraps this command, verifies the promoted image tags, validates the
  rendered overlay, and publishes a reviewable patch artifact.
- Replace the public host and ingress class for the target Ingress controller.
- Create or sync the `mpp-public-tls` Secret for the production public host.
- Replace managed PostgreSQL and Redis ExternalName targets with provider
  hostnames.
- Keep `DB_HOST` equal to the managed PostgreSQL provider hostname when
  `DB_SSLMODE=verify-full` so PostgreSQL certificate hostname verification
  succeeds.
- Keep `DB_READER_HOST` equal to the managed read replica provider hostname
  when reader connections inherit `DB_SSLMODE=verify-full`.
- Keep `REDIS_ADDR` equal to the managed Redis provider hostname and port when
  `REDIS_TLS=true` so Redis certificate hostname verification succeeds.
- Set `REDIS_TLS_CA_CERT` or `REDIS_TLS_CA_FILE` only when the provider requires
  custom trust material; prefer a mounted CA file for multiline bundles. Use
  `REDIS_TLS_SERVER_NAME` only when the provider documents an SNI name that
  differs from `REDIS_ADDR`.
- Record the production Redis persistence mode, snapshot retention, restore
  point objective, and provider restore procedure with the production change
  record before applying this overlay.
- Replace `LLM_MODEL=replace-with-production-model` with the provider model
  used by production.
- Replace `X_OAUTH2_CLIENT_ID` and keep `X_OAUTH2_REDIRECT_URL` aligned to the
  production public host.
- Install External Secrets Operator, create or patch the referenced
  `ClusterSecretStore`, and replace every `ExternalSecret` remote key with the
  production provider path. Add a `REDIS_PASSWORD` remote key when the managed
  Redis provider requires auth.
- For provider-specific overlays, keep the shared runtime, app, and data-service
  resources from this overlay and patch only provider hostnames, TLS/ingress
  details, secret-store bindings, CA mounts, and immutable image tags.

Render and validate:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/production-managed > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/production-managed \
  "$rendered"
```

After replacing the example inputs, run deployable validation. This rejects
`.example.invalid` hosts, all-zero SHA image tags, and example model values:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/production-managed > "$rendered"
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/production-managed \
  "$rendered"
```

Apply only after the External Secrets Operator store exists and the example
inputs are replaced:

```bash
kubectl apply -k deploy/kubernetes/overlays/production-managed
```
