# Non-Production HA Redis

This package deploys a parallel Redis HA topology for non-production validation:

- `redis-ha-primary`: one Redis primary StatefulSet with persistent storage.
- `redis-ha-replica`: two Redis replica Pods that replicate from the primary.
- `redis-ha-sentinel`: three Redis Sentinel Pods with quorum `2`.

It intentionally does not create or change the `redis` Service used by
application workloads. Non-production apps keep using
`REDIS_ENDPOINT_MODE=direct` and `REDIS_ADDR=redis:6379` unless the app
ConfigMap is explicitly switched to the Sentinel endpoint.

Apply it by including this package from a non-production overlay, such as
`deploy/kubernetes/overlays/staging-self-hosted`. Do not include it in managed
or production overlays.

Validation checks:

```bash
kubectl rollout status statefulset/redis-ha-primary -n "$MPP_APP_NS"
kubectl rollout status statefulset/redis-ha-replica -n "$MPP_APP_NS"
kubectl rollout status statefulset/redis-ha-sentinel -n "$MPP_APP_NS"
kubectl get svc -n "$MPP_APP_NS" redis redis-ha-primary redis-ha-replicas redis-ha-sentinel
```

Replication health:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-replica -- sh -ec '
  redis_cli() {
    if [ -n "${REDIS_PASSWORD:-}" ]; then
      redis-cli --raw --no-auth-warning -a "$REDIS_PASSWORD" "$@"
    else
      redis-cli --raw "$@"
    fi
  }
  redis_cli INFO replication | tr -d "\r" | grep -E "role:|master_link_status:"
'
```

Sentinel health:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-sentinel -- sh -ec '
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mpp-redis-ha
  redis-cli -p 26379 SENTINEL ckquorum mpp-redis-ha
'
```

Application endpoint switch:

```yaml
REDIS_ENDPOINT_MODE: sentinel
REDIS_SENTINEL_ADDRS: redis-ha-sentinel:26379
REDIS_SENTINEL_MASTER_NAME: mpp-redis-ha
REDIS_ADDR: redis:6379
```

`REDIS_ADDR` stays as the direct-mode rollback endpoint. To switch back, set
`REDIS_ENDPOINT_MODE=direct` and confirm `REDIS_ADDR=redis:6379`.

After switching non-production app Pods to Sentinel mode, run the client
failover drill:

```bash
MPP_APP_NS=mpp-system script/kubernetes/redis-ha-failover-drill.sh
```

The drill triggers Sentinel failover, waits for a new master, verifies
`backend`, `publish-worker`, and `browser-worker` readiness, writes a
verification-code key through `backend`, confirms a second request reads the
rate-limit key, and prints the observed recovery time. The Phase 2 target is
recovery within 300 seconds.

Rollback is intentionally simple: switch the app ConfigMap back to direct mode,
remove this package from the non-production overlay if the HA topology itself
must be rolled back, apply the overlay again, and delete leftover `redis-ha-*`
PVCs only after the validation data is no longer useful.
