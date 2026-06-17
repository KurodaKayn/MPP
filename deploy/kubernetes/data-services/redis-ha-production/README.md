# Production HA Redis

This package is the production-eligible wrapper for the Phase 2 self-hosted HA
Redis topology. It reuses the validated primary, replica, and Sentinel
manifests from `deploy/kubernetes/data-services/redis-ha-nonprod`, then opens
the HA Redis NetworkPolicy to `redis-exporter` so the production cutover can
monitor the HA target.

Use this package only after the non-production failover drill and migration
rehearsal have passed. It can be applied before the endpoint switch to create
the HA target while application traffic still uses `REDIS_ENDPOINT_MODE=direct`
and `REDIS_ADDR=redis:6379`. It does not remove the existing `redis` Service or
StatefulSet; production cutover keeps that direct endpoint available as the
rollback path until the soak window is closed.

Production app traffic should use:

```yaml
REDIS_ENDPOINT_MODE: sentinel
REDIS_SENTINEL_ADDRS: redis-ha-sentinel:26379
REDIS_SENTINEL_MASTER_NAME: mpp-redis-ha
REDIS_ADDR: redis:6379
REDIS_TLS: "false"
```

Before applying this package or a production overlay that includes it:

- Confirm the source Redis snapshot or backup for the cutover window exists.
- Confirm `mpp-app-secrets` includes `REDIS_PASSWORD` for self-hosted Redis
  auth.
- Run the production prechecks in `doc/kubernetes-operations-runbook.md`.
- Migrate Redis data while apps still use `REDIS_ENDPOINT_MODE=direct`.
- Apply the endpoint switch only inside the approved maintenance window.

Render and validate:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/data-services/redis-ha-production > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/data-services/redis-ha-production \
  "$rendered"
```

Prepare the HA target before final data migration:

```bash
kubectl apply -k deploy/kubernetes/data-services/redis-ha-production
```
