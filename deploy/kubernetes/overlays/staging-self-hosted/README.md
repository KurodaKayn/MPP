# Staging Self-Hosted Overlay

This overlay is a renderable staging starter for clusters that run PostgreSQL
and Redis in-cluster. It combines:

- `deploy/kubernetes/browser-runtime-control`
- `deploy/kubernetes/app-baseline`
- `deploy/kubernetes/data-services/self-hosted`

The checked-in values are intentionally non-production:

- App and browser runtime images use the immutable-looking
  `sha-0000000000000000000000000000000000000000` example tag.
- The public host is `mpp-staging.example.invalid`.
- `mpp-app-secrets` is generated from example literals so the overlay can
  render and validate without committing real credentials.
- PostgreSQL TLS is disabled because this overlay uses the self-hosted
  PostgreSQL Service.
- App Pods connect to PostgreSQL through the in-cluster PgBouncer writer pool.

Before applying this overlay to a shared staging cluster:

- Replace every `sha-0000000000000000000000000000000000000000` image tag with
  registry-published `sha-<full-git-sha>` tags.
- Replace the public host and TLS Secret inputs for the target Ingress
  controller.
- Replace every generated Secret literal through your staging secret workflow;
  `ruby script/kubernetes/render-app-secret.rb --require-redis-password` can
  render the `mpp-app-secrets` manifest from a temporary env file.
- Patch storage classes, storage sizes, PgBouncer pool sizing, and data-service
  resource limits if the cluster defaults are not appropriate.
- Patch the `mpp-data-backups` PVC, `postgres-backup` and `redis-backup`
  schedules, and `BACKUP_RETENTION_DAYS` before keeping useful staging data in
  the StatefulSets.
- Copy backup artifacts off the in-cluster backup PVC or enable
  storage-provider snapshots before treating this overlay as recoverable.

Render and validate:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/staging-self-hosted > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-self-hosted \
  "$rendered"
```

After replacing the example inputs, run deployable validation. This rejects
`.example.invalid` hosts, all-zero SHA image tags, and generated example Secret
values:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/staging-self-hosted > "$rendered"
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-self-hosted \
  "$rendered"
```

Apply only after replacing the example inputs:

```bash
kubectl apply -k deploy/kubernetes/overlays/staging-self-hosted
```
