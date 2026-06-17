# Non-Production HA Redis

This package deploys a parallel Redis HA topology for non-production validation:

- `redis-ha-primary`: one Redis primary StatefulSet with persistent storage.
- `redis-ha-replica`: two Redis replica Pods that replicate from the primary.
- `redis-ha-sentinel`: three Redis Sentinel Pods with quorum `2`.

It intentionally does not create or change the `redis` Service used by
application workloads. Non-production apps keep using `REDIS_ADDR=redis:6379`
until a later endpoint-abstraction and cutover PR points traffic elsewhere.

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

Rollback is intentionally simple: remove this package from the non-production
overlay, apply the overlay again, and delete leftover `redis-ha-*` PVCs if the
validation data is no longer useful. Application workloads continue to use the
existing `redis` Service during and after rollback.
