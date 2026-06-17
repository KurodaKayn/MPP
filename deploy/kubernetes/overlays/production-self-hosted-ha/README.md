# Production Self-Hosted HA Overlay

This overlay is the Phase 2 production endpoint-switch overlay for clusters moving from
the existing self-hosted Redis StatefulSet to the self-hosted HA Redis topology.
It combines:

- `deploy/kubernetes/browser-runtime-control`
- `deploy/kubernetes/app-baseline`
- `deploy/kubernetes/data-services/self-hosted`
- `deploy/kubernetes/data-services/redis-ha-production`
- `deploy/kubernetes/external-secrets`

The checked-in values are renderable placeholders only:

- App and browser runtime images use the immutable-looking
  `sha-0000000000000000000000000000000000000000` example tag.
- The public host is `mpp.example.invalid`.
- `mpp-app-secrets` is generated only at runtime by External Secrets Operator.
  This overlay patches the ExternalSecret contract to include `REDIS_PASSWORD`
  because production self-hosted Redis must use auth.
- PostgreSQL and Redis run in-cluster from the self-hosted data-services base.
- App Redis clients use Sentinel mode after cutover while keeping
  `REDIS_ADDR=redis:6379` as the direct rollback endpoint.

Before applying this overlay to production:

- Complete the non-production HA failover validation and migration rehearsal.
- Pin all app and browser runtime images with
  `ruby script/kubernetes/pin-overlay-images.rb --overlay deploy/kubernetes/overlays/production-self-hosted-ha --git-sha <full-git-sha>`.
- Replace the public host, ingress class, TLS Secret, production LLM endpoint,
  model name, object-storage values, and OAuth client settings.
- Install External Secrets Operator, create or patch the referenced
  `ClusterSecretStore`, and replace every `ExternalSecret` remote key with the
  production provider path.
- Confirm `REDIS_PASSWORD` is materialized in `mpp-app-secrets` before starting
  self-hosted Redis or HA Redis Pods.
- Record the source Redis snapshot or backup, migration report path, rollback
  owner, and approved maintenance window in the production change record.

Render and validate:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/production-self-hosted-ha > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/production-self-hosted-ha \
  "$rendered"
```

After replacing the example inputs, run deployable validation. This rejects
`.example.invalid` hosts, all-zero SHA image tags, and example model values:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/production-self-hosted-ha > "$rendered"
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/production-self-hosted-ha \
  "$rendered"
```

Apply only inside the approved production cutover window and only after the
HA target has been prepared with `deploy/kubernetes/data-services/redis-ha-production`
and source Redis data has been migrated into `redis-ha-primary`:

```bash
kubectl apply -k deploy/kubernetes/overlays/production-self-hosted-ha
kubectl rollout restart deployment/backend deployment/publish-worker \
  deployment/browser-worker deployment/collab-service -n mpp-system
```
