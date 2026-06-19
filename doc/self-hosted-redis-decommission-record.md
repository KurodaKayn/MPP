# Self-Hosted Redis Decommission Record

Use this record for issue #339 after the managed Redis production soak from
issue #338 is accepted. It preserves the evidence needed to prove the old
self-hosted Redis deployment receives no production traffic, keeps a restorable
backup for the agreed retention window, and records how to recreate the old
chart from Git history if rollback is required.

Do not paste Redis passwords, provider secrets, private keys, or raw credential
material into this file.

## Execution Summary

| Field | Value |
| --- | --- |
| Issue | #339 |
| Blocker | #338 closed |
| Active production overlay | `deploy/kubernetes/overlays/production-managed` |
| Retired overlay marker | `deploy/kubernetes/overlays/production-self-hosted-ha` renders only `ConfigMap/production-self-hosted-ha-retired` |
| Retired Redis HA package marker | `deploy/kubernetes/data-services/redis-ha-production` renders only `ConfigMap/redis-ha-production-retired` |
| Execution status | Pending production execution |
| Decommission window start | Pending |
| Decommission window end | Pending |
| Operator | Pending |
| Rollback owner | Pending |
| Retention window end | Pending |
| Previous self-hosted overlay Git SHA | Pending |
| Retained snapshot or backup ID | Pending |
| Change record link | Pending |

## Completion Gates

| Gate | Evidence required | Status |
| --- | --- | --- |
| No old traffic | Managed production config is active, Redis-dependent workloads no longer use Sentinel, and old self-hosted Redis Pods receive no app or exporter connections | Pending |
| Writes stopped | The old self-hosted Redis primaries are quiescent, publish/browser/collab workloads have no Redis connections to old Redis Pods, and write counters stay flat during the observation window | Pending |
| Backup retained | A restorable self-hosted Redis RDB or storage snapshot is retained outside deleted Pods/PVCs until the retention window end | Pending |
| Deployment removed | Old self-hosted Redis HA StatefulSets, Services, NetworkPolicies, exporter wiring, and PVCs are deleted or disabled after no-traffic evidence is accepted | Pending |
| Rollback documented | Previous chart Git SHA, render command, snapshot restore procedure, owner, and retention deadline are recorded | Pending |

## Pre-Decommission Checks

Set the shell context for every command block:

```bash
export MPP_APP_NS=mpp-system
export SOURCE_OVERLAY=deploy/kubernetes/overlays/production-self-hosted-ha
export ACTIVE_OVERLAY=deploy/kubernetes/overlays/production-managed
export RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
export ARTIFACT_DIR="artifacts/redis-self-hosted-decommission/${RUN_ID}"
mkdir -p "$ARTIFACT_DIR"
```

Confirm the managed production overlay is the active desired state:

```bash
rendered="$(mktemp)"
kubectl kustomize "$ACTIVE_OVERLAY" > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb "$ACTIVE_OVERLAY" "$rendered"
cp "$rendered" "$ARTIFACT_DIR/production-managed-rendered.yaml"
```

Confirm retired production self-hosted packages cannot recreate Redis from the
current branch:

```bash
for package in \
  deploy/kubernetes/overlays/production-self-hosted-ha \
  deploy/kubernetes/data-services/redis-ha-production
do
  rendered="$(mktemp)"
  kubectl kustomize "$package" > "$rendered"
  ruby script/kubernetes/validate-rendered-manifests.rb "$package" "$rendered"
  cp "$rendered" "$ARTIFACT_DIR/$(basename "$package")-retired-rendered.yaml"
done
```

Confirm running app config points at managed Redis and not Sentinel:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config \
  -o jsonpath='{.data.APP_ENV}{" REDIS_ENDPOINT_MODE="}{.data.REDIS_ENDPOINT_MODE}{" REDIS_ADDR="}{.data.REDIS_ADDR}{" REDIS_TLS="}{.data.REDIS_TLS}{" REDIS_SENTINEL_ADDRS="}{.data.REDIS_SENTINEL_ADDRS}{"\n"}' \
  | tee "$ARTIFACT_DIR/managed-active-config.txt"
```

Expected result: `APP_ENV=production`, `REDIS_ENDPOINT_MODE=direct` or empty
direct-mode default, `REDIS_ADDR` is the managed provider hostname and port, and
`REDIS_SENTINEL_ADDRS` is empty or no longer used by the running clients.

Confirm old Redis has no production traffic before decommissioning. Check both
the self-hosted HA primary and the earlier direct Redis StatefulSet when they
still exist:

```bash
for target in statefulset/redis-ha-primary statefulset/redis
do
  kubectl exec -n "$MPP_APP_NS" "$target" -c redis -- sh -ec '
    if [ -n "${REDIS_PASSWORD:-}" ]; then
      export REDISCLI_AUTH="$REDIS_PASSWORD"
    fi
    redis-cli --raw --no-auth-warning INFO clients
    redis-cli --raw --no-auth-warning INFO commandstats
  ' | tee "$ARTIFACT_DIR/$(echo "$target" | tr / -)-traffic-before.txt" || true
done

sleep 300

for target in statefulset/redis-ha-primary statefulset/redis
do
  kubectl exec -n "$MPP_APP_NS" "$target" -c redis -- sh -ec '
    if [ -n "${REDIS_PASSWORD:-}" ]; then
      export REDISCLI_AUTH="$REDIS_PASSWORD"
    fi
    redis-cli --raw --no-auth-warning INFO clients
    redis-cli --raw --no-auth-warning INFO commandstats
  ' | tee "$ARTIFACT_DIR/$(echo "$target" | tr / -)-traffic-after.txt" || true
done
```

Expected result: no app client connections to `redis-ha-*`, no Redis exporter
or Redis backup CronJob scraping old Redis Pods, and no command counters
increasing beyond operator inspection commands during the observation window.

## Retained Backup

Take or confirm the final self-hosted Redis backup before deleting resources:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-primary -c redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
  fi
  redis-cli --raw --no-auth-warning SAVE
  redis-cli --raw --no-auth-warning LASTSAVE
' | tee "$ARTIFACT_DIR/final-self-hosted-lastsave.txt"
```

Retain one of these artifacts until the retention window end:

| Artifact | Required evidence |
| --- | --- |
| Redis RDB backup | Backup object path, checksum, size, creation time, restore target, and retention expiration |
| Storage snapshot | Provider snapshot ID, PVC/source volume, creation time, restore target, and retention expiration |
| Migration artifact | Source inventory and copy report from issue #338 when accepted as rollback evidence |

Record the retained artifact:

| Field | Value |
| --- | --- |
| Backup type | Pending |
| Backup ID or path | Pending |
| SHA256 or provider checksum | Pending |
| Size | Pending |
| Created at | Pending |
| Retention expires at | Pending |
| Restore target | Pending |
| Restore owner | Pending |
| Verification command output | Pending |

## Remove Old Deployment

Run removal only after the no-traffic and backup-retention gates are accepted.
Do not delete `Service/redis` directly when the managed overlay owns it as an
ExternalName alias; reapply the active managed overlay instead.

```bash
kubectl delete statefulset -n "$MPP_APP_NS" \
  redis redis-ha-primary redis-ha-replica redis-ha-sentinel --ignore-not-found

kubectl delete service -n "$MPP_APP_NS" \
  redis-ha-primary redis-ha-primary-headless \
  redis-ha-replicas redis-ha-replicas-headless \
  redis-ha-sentinel redis-ha-sentinel-headless --ignore-not-found

kubectl delete cronjob -n "$MPP_APP_NS" \
  redis-backup --ignore-not-found

kubectl delete configmap -n "$MPP_APP_NS" \
  redis-persistence-config --ignore-not-found

kubectl delete networkpolicy -n "$MPP_APP_NS" \
  redis-app-access redis-ha-internal-access --ignore-not-found

kubectl apply -k "$ACTIVE_OVERLAY"
```

Delete old self-hosted Redis PVCs only after confirming the retained backup or
snapshot can be restored independently of those PVCs:

```bash
kubectl delete pvc -n "$MPP_APP_NS" \
  -l 'app.kubernetes.io/component in (redis,redis-ha-primary,redis-ha-replica,redis-ha-sentinel)' \
  --ignore-not-found
```

Confirm only managed Redis resources remain in the active production path:

```bash
kubectl get statefulset,svc,pvc,networkpolicy -n "$MPP_APP_NS" \
  | grep -E 'redis-ha|redis ' \
  | tee "$ARTIFACT_DIR/post-delete-redis-resources.txt" || true
```

Expected result: no old `redis` or `redis-ha-*` StatefulSets, old Redis PVCs,
Redis backup CronJob, or old Redis NetworkPolicies remain. A `redis`
ExternalName Service may remain when the managed overlay uses it as a stable
in-cluster alias for the provider endpoint.

## Rollback During Retention Window

Rollback is chart recreation plus snapshot restore. It is slower than the
pre-decommission endpoint rollback and requires rollback-owner approval.

1. Check out the previous self-hosted overlay Git SHA recorded in this file.
2. Render `deploy/kubernetes/overlays/production-self-hosted-ha` from that
   historical revision and validate it before applying.
3. Apply the historical overlay to recreate the self-hosted HA Redis chart.
4. Restore the retained RDB backup or storage snapshot into the recreated
   `redis-ha-primary` data path.
5. Confirm `PING`, `INFO keyspace`, Sentinel quorum, and sampled critical keys.
6. Patch application config back to Sentinel mode only after restore acceptance:

```bash
kubectl patch configmap mpp-app-config -n "$MPP_APP_NS" --type merge -p \
  '{"data":{"REDIS_ENDPOINT_MODE":"sentinel","REDIS_SENTINEL_ADDRS":"redis-ha-sentinel:26379","REDIS_SENTINEL_MASTER_NAME":"mpp-redis-ha","REDIS_ADDR":"redis:6379","REDIS_TLS":"false","REDIS_TLS_CA_FILE":"","REDIS_TLS_CA_CERT":"","REDIS_TLS_SERVER_NAME":""}}'

kubectl rollout restart deployment/backend deployment/publish-worker \
  deployment/browser-worker deployment/collab-service -n "$MPP_APP_NS"
```

Do not copy data back from managed Redis unless the incident commander accepts
the risk of overwriting or reintroducing bad state. Prefer restoring the
retained self-hosted snapshot into an isolated target first, then decide whether
traffic rollback is still required.

## Closeout

| Check | Evidence |
| --- | --- |
| Managed config active after deletion | Pending |
| Old Redis traffic remained zero | Pending |
| Backup retained through retention window | Pending |
| Old Redis StatefulSets deleted or disabled | Pending |
| Old Redis backup CronJob deleted or disabled | Pending |
| Old Redis Services and NetworkPolicies deleted or disabled | Pending |
| PVC deletion approved or explicitly deferred | Pending |
| Rollback notes reviewed by owner | Pending |
| Follow-up issues | Pending |
