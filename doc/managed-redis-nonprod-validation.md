# Managed Redis Non-Production Validation Report

Use this report for Phase 4.2 before any production traffic moves from
self-hosted HA Redis to a managed Redis HA provider. The validation must run in
a non-production environment that uses `deploy/kubernetes/overlays/staging-managed`
or a provider-specific overlay derived from it.

Do not use this report as a production cutover runbook. It proves provider
behavior, rollback boundaries, and operational visibility only.

## Validation Status

| Field | Value |
| --- | --- |
| Issue | #336 |
| Environment | Non-production managed Redis |
| Provider and plan | To be recorded during validation |
| Managed endpoint | To be recorded during validation |
| Validation window | To be recorded during validation |
| Operator | To be recorded during validation |
| Rollback owner | To be recorded during validation |
| Overall result | Pending provider execution |

## Preconditions

- #335 is complete, so `REDIS_ADDR`, `REDIS_TLS`, `REDIS_PASSWORD`,
  `REDIS_DB`, `REDIS_TLS_CA_CERT`, `REDIS_TLS_CA_FILE`, and
  `REDIS_TLS_SERVER_NAME` can select the managed provider without code changes.
- The managed Redis instance is non-production, private to the staging network,
  and has no production traffic or production data.
- `deploy/kubernetes/overlays/staging-managed` has provider hostnames, image
  tags, ingress host, TLS settings, and staging secrets replaced.
- The managed provider persistence policy, snapshot retention, restore target
  behavior, maintenance window, and documented RPO are available before testing.
- `deploy/kubernetes/overlays/staging-self-hosted` or the self-hosted HA
  rollback endpoint remains available and unchanged.
- Redis exporter is pointed at the managed endpoint, using `rediss://` when
  `REDIS_TLS=true`.

## Setup And Rollback Boundary

Render and validate the managed staging overlay:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/staging-managed > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-managed \
  "$rendered"

MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-managed \
  "$rendered"
```

Apply only after deployable validation passes:

```bash
kubectl apply -k deploy/kubernetes/overlays/staging-managed
kubectl rollout restart deployment/backend deployment/publish-worker \
  deployment/browser-worker deployment/collab-service -n "$MPP_APP_NS"
kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/browser-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/collab-service -n "$MPP_APP_NS" --timeout=5m
```

Confirm that the running config selects the managed endpoint:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config \
  -o jsonpath='{.data.APP_ENV}{" REDIS_ENDPOINT_MODE="}{.data.REDIS_ENDPOINT_MODE}{" REDIS_ADDR="}{.data.REDIS_ADDR}{" REDIS_TLS="}{.data.REDIS_TLS}{"\n"}'
```

Rollback must not depend on deleting or mutating the managed instance. Roll back
application traffic by applying the previous self-hosted staging overlay, or by
restoring the self-hosted HA Redis settings:

```bash
kubectl patch configmap mpp-app-config -n "$MPP_APP_NS" --type merge -p \
  '{"data":{"REDIS_ENDPOINT_MODE":"sentinel","REDIS_SENTINEL_ADDRS":"redis-ha-sentinel:26379","REDIS_SENTINEL_MASTER_NAME":"mpp-redis-ha","REDIS_ADDR":"redis:6379","REDIS_TLS":"false"}}'
kubectl rollout restart deployment/backend deployment/publish-worker \
  deployment/browser-worker deployment/collab-service -n "$MPP_APP_NS"
```

## Validation Matrix

| Area | Required evidence | Result |
| --- | --- | --- |
| TLS and auth | Authenticated TLS `PING` succeeds; unauthenticated access fails; non-TLS access fails when provider requires TLS; CA or SNI requirements are recorded | Pending |
| Latency | Baseline p50/p95/p99 from `redis-cli --latency-history` and `redis_latency_percentiles_usec`; application Redis error/fallback counters remain normal | Pending |
| Failover | Provider failover is triggered in non-production; app readiness and Redis write/read probe recover; observed RTO is recorded | Pending |
| Backup restore | Snapshot or backup is restored into an isolated non-production target; durable probe key is present; expected short-TTL loss is documented; observed RTO/RPO are recorded | Pending |
| Metrics and logs | Redis exporter, Prometheus alerts, provider metrics, provider event logs, and app `mpp_redis_*` metrics are visible during normal and failover windows | Pending |
| Tear-down | Managed test resources can be deleted without affecting self-hosted HA Redis or its PVCs | Pending |
| Production risks | Provider limitations and required production settings are listed before cutover planning | Pending |

## TLS And Auth

Capture the active Redis config without printing secret values:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config \
  -o jsonpath='{.data.REDIS_ADDR}{" REDIS_TLS="}{.data.REDIS_TLS}{" REDIS_TLS_CA_FILE="}{.data.REDIS_TLS_CA_FILE}{" REDIS_TLS_SERVER_NAME="}{.data.REDIS_TLS_SERVER_NAME}{"\n"}'
kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets \
  -o jsonpath='{.data.REDIS_PASSWORD}' | wc -c
```

Run a TLS/auth probe from inside the cluster network. Use the provider hostname
from `REDIS_ADDR`, not the `redis` ExternalName alias, when TLS hostname
verification is enabled.

```bash
REDIS_ADDR="$(kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o jsonpath='{.data.REDIS_ADDR}')"

kubectl run redis-provider-probe -n "$MPP_APP_NS" --rm -i --restart=Never \
  --image=redis:7.2-alpine \
  --env="REDIS_ADDR=$REDIS_ADDR" \
  --overrides='{"spec":{"containers":[{"name":"redis-provider-probe","image":"redis:7.2-alpine","env":[{"name":"REDIS_PASSWORD","valueFrom":{"secretKeyRef":{"name":"mpp-app-secrets","key":"REDIS_PASSWORD"}}}]}]}}' \
  -- sh -ec '
    host="${REDIS_ADDR%:*}"
    port="${REDIS_ADDR##*:}"
    export REDISCLI_AUTH="$REDIS_PASSWORD"
    redis-cli --tls -h "$host" -p "$port" --no-auth-warning PING
  '
```

Record:

| Check | Observed result |
| --- | --- |
| Authenticated TLS ping | Pending |
| Unauthenticated ping behavior | Pending |
| Non-TLS ping behavior | Pending |
| CA source | Pending |
| SNI or certificate hostname requirement | Pending |

## Latency Baseline

Run the latency probe for at least five minutes during a normal staging window:

```bash
REDIS_ADDR="$(kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o jsonpath='{.data.REDIS_ADDR}')"

kubectl run redis-latency-probe -n "$MPP_APP_NS" --rm -i --restart=Never \
  --image=redis:7.2-alpine \
  --env="REDIS_ADDR=$REDIS_ADDR" \
  --overrides='{"spec":{"containers":[{"name":"redis-latency-probe","image":"redis:7.2-alpine","env":[{"name":"REDIS_PASSWORD","valueFrom":{"secretKeyRef":{"name":"mpp-app-secrets","key":"REDIS_PASSWORD"}}}]}]}}' \
  -- sh -ec '
    host="${REDIS_ADDR%:*}"
    port="${REDIS_ADDR##*:}"
    export REDISCLI_AUTH="$REDIS_PASSWORD"
    timeout 300 redis-cli --tls -h "$host" -p "$port" --no-auth-warning --latency-history
  '
```

Prometheus evidence:

```promql
max(redis_up{service="redis"})
max by (cmd, quantile) (redis_latency_percentiles_usec{service="redis",quantile=~"0.95|0.99"}) / 1000000
sum(rate(mpp_redis_operations_total{status="error"}[5m])) or vector(0)
sum(rate(mpp_redis_fallback_total[5m])) or vector(0)
```

Record:

| Metric | Observed value |
| --- | --- |
| Redis p50 latency | Pending |
| Redis p95 latency | Pending |
| Redis p99 latency | Pending |
| App Redis error rate during baseline | Pending |
| App Redis fallback rate during baseline | Pending |
| Provider latency dashboard link or export | Pending |

## Failover Behavior

Use only the provider's documented non-production failover action. Do not delete
Kubernetes Pods for this test; the Redis primary is outside the cluster.

Start the timer immediately before triggering provider failover:

```bash
backend_pod="$(kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=backend -o jsonpath='{.items[0].metadata.name}')"
publish_worker_pod="$(kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=publish-worker -o jsonpath='{.items[0].metadata.name}')"
browser_worker_pod="$(kubectl get pod -n "$MPP_APP_NS" -l app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-worker -o jsonpath='{.items[0].metadata.name}')"

start_epoch="$(date +%s)"
# Trigger the provider-managed Redis failover in the provider console or API.

until kubectl exec -n "$MPP_APP_NS" "$backend_pod" -c backend -- wget -qO- -T 10 http://127.0.0.1:8080/ready | grep -q '"status":"ready"' && \
  kubectl exec -n "$MPP_APP_NS" "$publish_worker_pod" -c publish-worker -- wget -qO- -T 10 http://127.0.0.1:8080/ready | grep -q '"status":"ready"' && \
  kubectl exec -n "$MPP_APP_NS" "$browser_worker_pod" -c browser-worker -- wget -qO- -T 10 http://127.0.0.1:8081/ready | grep -q '"status":"ready"'; do
  sleep 5
done

echo "observed_rto_seconds=$(( $(date +%s) - start_epoch ))"
```

After readiness returns, run one Redis-backed app write/read probe. The existing
self-hosted Sentinel drill uses `/api/auth/send-code`; use the same flow if the
staging email path is safe, and record the generated test email in the change
record.

Record:

| Field | Observed value |
| --- | --- |
| Provider failover method | Pending |
| Failover start timestamp | Pending |
| Failover complete timestamp | Pending |
| Observed RTO seconds | Pending |
| App readiness behavior | Pending |
| Backend Redis write/read probe | Pending |
| Redis errors or circuit-breaker transitions | Pending |
| Provider failover event log | Pending |

## Backup Restore

Restore into a separate isolated non-production Redis instance. Never restore a
snapshot over the live staging writer instance.

Before the provider snapshot, create durable and short-TTL probes:

```bash
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
REDIS_ADDR="$(kubectl get configmap -n "$MPP_APP_NS" mpp-app-config -o jsonpath='{.data.REDIS_ADDR}')"

kubectl run redis-restore-probe -n "$MPP_APP_NS" --rm -i --restart=Never \
  --image=redis:7.2-alpine \
  --env="REDIS_ADDR=$REDIS_ADDR" \
  --env="RUN_ID=$RUN_ID" \
  --overrides='{"spec":{"containers":[{"name":"redis-restore-probe","image":"redis:7.2-alpine","env":[{"name":"REDIS_PASSWORD","valueFrom":{"secretKeyRef":{"name":"mpp-app-secrets","key":"REDIS_PASSWORD"}}}]}]}}' \
  -- sh -ec '
    host="${REDIS_ADDR%:*}"
    port="${REDIS_ADDR##*:}"
    export REDISCLI_AUTH="$REDIS_PASSWORD"
    redis-cli --tls -h "$host" -p "$port" --no-auth-warning SET "mpp:managed-validation:persistent:${RUN_ID}" "present"
    redis-cli --tls -h "$host" -p "$port" --no-auth-warning SETEX "mpp:managed-validation:short-ttl:${RUN_ID}" 30 "expires"
  '
```

Take or select the provider snapshot, restore it to the isolated target, and
verify keys there. The durable key should exist when the snapshot includes it.
The short-TTL key may be absent if it expired before restore verification.

Record:

| Field | Observed value |
| --- | --- |
| Snapshot identifier | Pending |
| Snapshot timestamp | Pending |
| Last accepted write included in snapshot | Pending |
| Restore target endpoint | Pending |
| Restore start timestamp | Pending |
| Restore verification timestamp | Pending |
| Observed restore RTO seconds | Pending |
| Observed restore RPO | Pending |
| Durable probe key result | Pending |
| Short-TTL probe key result | Pending |
| Restore limitations | Pending |

## Metrics And Logs

Confirm these signals during baseline and failover windows:

| Signal | Source | Observed result |
| --- | --- | --- |
| `redis_up` | Redis exporter | Pending |
| `redis_latency_percentiles_usec` | Redis exporter | Pending |
| `redis_connected_clients` | Redis exporter | Pending |
| `redis_blocked_clients` | Redis exporter | Pending |
| `redis_memory_used_bytes` and `redis_memory_max_bytes` | Redis exporter | Pending |
| `redis_evicted_keys_total` | Redis exporter | Pending |
| `mpp_redis_operations_total` | App metrics | Pending |
| `mpp_redis_errors_total` | App metrics | Pending |
| `mpp_redis_fallback_total` | App metrics | Pending |
| Provider failover events | Provider console or API | Pending |
| Provider backup and restore events | Provider console or API | Pending |

If `redis_config_maxclients` or other exporter metrics are missing because the
provider restricts commands, record the missing metrics and the compensating
provider dashboard or alert.

## Tear-Down Check

After validation, tear down only the managed test resources and any temporary
probe Pods. Confirm self-hosted HA resources and PVCs are untouched:

```bash
kubectl get statefulset,pod,svc,pvc -n "$MPP_APP_NS" | grep -E 'redis-ha|redis-provider-probe|redis-latency-probe|redis-restore-probe' || true
```

Record:

| Check | Observed result |
| --- | --- |
| Temporary probe Pods removed | Pending |
| Managed restore target removed or retained with owner/date | Pending |
| Self-hosted HA StatefulSets unchanged | Pending |
| Self-hosted HA PVCs unchanged | Pending |

## Production Cutover Risks And Required Settings

Record open items before Phase 4.3 production migration runbook work starts:

| Area | Required production decision or risk |
| --- | --- |
| Endpoint and DNS | Pending |
| TLS CA and SNI | Pending |
| Redis auth and rotation | Pending |
| Database index support | Pending |
| Snapshot retention and RPO | Pending |
| Restore target behavior | Pending |
| Provider failover RTO | Pending |
| Maintenance window and version upgrades | Pending |
| Eviction and maxmemory policy | Pending |
| Metrics or command visibility gaps | Pending |
| Scaling path to Redis Cluster | Pending |
| Rollback to self-hosted HA | Pending |

Do not proceed to cutover planning until this table has owners and accepted
answers.
