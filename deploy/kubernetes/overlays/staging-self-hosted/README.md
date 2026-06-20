# Staging Self-Hosted Overlay

This overlay is a renderable staging starter for clusters that run PostgreSQL
and Redis in-cluster. It combines:

- `deploy/kubernetes/browser-runtime-control`
- `deploy/kubernetes/app-baseline`
- `deploy/kubernetes/data-services/self-hosted`
- `deploy/kubernetes/data-services/redis-ha-nonprod`

The checked-in values are intentionally non-production:

- App and browser runtime images use the immutable-looking
  `sha-0000000000000000000000000000000000000000` example tag.
- The public host is `mpp-staging.example.invalid`.
- `mpp-app-secrets` is generated from example literals so the overlay can
  render and validate without committing real credentials.
- PostgreSQL TLS is disabled because this overlay uses the self-hosted
  PostgreSQL Service.
- App Pods connect to PostgreSQL through the in-cluster PgBouncer writer pool.
- Redis uses the self-hosted StatefulSet persistence baseline: the
  `redis-data` PVC stores `/data`, AOF is enabled with `appendfsync everysec`,
  and RDB snapshots keep the base `900/1`, `300/10`, and `60/10000` save
  cadence. It also inherits `maxmemory 384mb`, `maxmemory-policy noeviction`,
  `timeout 0`, `tcp-keepalive 300`, and bounded slowlog retention from the
  self-hosted data-services base.
- HA Redis validation resources deploy beside the existing Redis instance as
  `redis-ha-primary`, `redis-ha-replica`, and `redis-ha-sentinel`. The checked-in
  overlay keeps `REDIS_ENDPOINT_MODE=direct` and `REDIS_ADDR=redis:6379`, so app
  traffic stays on the existing single-instance Redis while the HA topology is
  validated.
- The non-production Redis Cluster package deploys a six-node TLS/auth cluster
  beside the existing Redis instance, along with bootstrap, backup, and
  exporter resources. It keeps the default app traffic on `redis:6379` until a
  deliberate cluster cutover is rehearsed.

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
- Patch `redis-persistence-config` only if staging intentionally chooses a
  different Redis data-loss profile. Document any change from the base
  AOF-plus-RDB policy in this file before applying it.
- To point app traffic at the non-production HA Redis endpoint, patch
  `mpp-app-config` to `REDIS_ENDPOINT_MODE=sentinel`,
  `REDIS_SENTINEL_ADDRS=redis-ha-sentinel:26379`, and
  `REDIS_SENTINEL_MASTER_NAME=mpp-redis-ha`. Keep `REDIS_ADDR=redis:6379` as the
  direct-mode rollback endpoint.
- To rehearse Redis Cluster traffic, patch `mpp-app-config` to
  `REDIS_ENDPOINT_MODE=cluster`, `REDIS_ADDR=redis-cluster.mpp-system.svc.cluster.local:6379`,
  and `REDIS_TLS=true`. Keep `REDIS_ADDR=redis:6379` as the direct rollback
  endpoint.
- To roll app traffic back from HA Redis, set `REDIS_ENDPOINT_MODE=direct` and
  confirm `REDIS_ADDR=redis:6379`; no business logic change is required.
- Remove `../../data-services/redis-ha-nonprod` from this overlay to roll back
  the parallel HA validation topology after app traffic is back on direct mode.
- Remove `../../data-services/redis-cluster-nonprod` from this overlay to roll
  back the Redis Cluster validation topology after app traffic is back on direct
  mode.
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
