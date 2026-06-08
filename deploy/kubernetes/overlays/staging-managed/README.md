# Staging Managed Overlay

This overlay is a renderable staging starter for clusters that use managed
PostgreSQL and Redis endpoints outside the cluster. It combines:

- `deploy/kubernetes/browser-runtime-control`
- `deploy/kubernetes/app-baseline`
- `deploy/kubernetes/data-services/managed`

The checked-in values are intentionally non-production:

- App and browser runtime images use the immutable-looking
  `sha-0000000000000000000000000000000000000000` example tag.
- The public host is `mpp-managed-staging.example.invalid`.
- Managed PostgreSQL and Redis ExternalName targets use `.example.invalid`
  provider hosts.
- `mpp-app-secrets` is generated from example literals so the overlay can
  render and validate without committing real credentials.

Before applying this overlay to a shared staging cluster:

- Replace every `sha-0000000000000000000000000000000000000000` image tag with
  registry-published `sha-<full-git-sha>` tags.
- Replace the public host and TLS Secret inputs for the target Ingress
  controller.
- Replace the managed PostgreSQL and Redis ExternalName targets with provider
  hostnames.
- Keep `DB_HOST` equal to the managed PostgreSQL provider hostname when
  `DB_SSLMODE=verify-full` so PostgreSQL certificate hostname verification
  succeeds.
- Keep `REDIS_ADDR` equal to the managed Redis provider hostname and port when
  `REDIS_TLS=true` so Redis certificate hostname verification succeeds.
- Replace every generated Secret literal through your staging secret workflow;
  `ruby script/kubernetes/render-app-secret.rb --require-redis-password` can
  render the `mpp-app-secrets` manifest from a temporary env file.

Render and validate:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/staging-managed > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-managed \
  "$rendered"
```

After replacing the example inputs, run deployable validation. This rejects
`.example.invalid` hosts, all-zero SHA image tags, and generated example Secret
values:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/staging-managed > "$rendered"
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-managed \
  "$rendered"
```

Apply only after replacing the example inputs:

```bash
kubectl apply -k deploy/kubernetes/overlays/staging-managed
```
