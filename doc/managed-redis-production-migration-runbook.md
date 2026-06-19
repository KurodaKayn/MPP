# Managed Redis Production Migration Runbook

Use this runbook to move production application traffic from self-hosted HA
Redis to a managed Redis HA provider. It is written for the Phase 4.3 migration
in issue #337 and assumes production already runs the self-hosted HA endpoint
from `deploy/kubernetes/overlays/production-self-hosted-ha`.

Use the managed Redis non-production validation evidence from
`doc/managed-redis-nonprod-validation.md` before executing the production
cutover. Production execution still requires accepted owner sign-off for the
selected provider and environment.

## Required Inputs

Record these values in the production change record before the window. Do not
paste secret values into the record.

| Input | Required value |
| --- | --- |
| Kubernetes context | Production cluster context confirmed by the operator and rollback owner |
| Namespaces | `MPP_APP_NS=mpp-system`, `MPP_OBS_NS=mpp-observability` unless the environment overrides them |
| Source Redis | Self-hosted HA Redis at `REDIS_ENDPOINT_MODE=sentinel`, `REDIS_SENTINEL_ADDRS=redis-ha-sentinel:26379`, `REDIS_SENTINEL_MASTER_NAME=mpp-redis-ha` |
| Managed Redis | Provider hostname and port used in `REDIS_ADDR`; `REDIS_TLS`, `REDIS_DB`, `REDIS_TLS_CA_FILE` or `REDIS_TLS_CA_CERT`, and `REDIS_TLS_SERVER_NAME` if the provider requires them |
| Secrets | `mpp-app-secrets` contains `REDIS_PASSWORD` only when the provider or source requires auth; External Secrets remote key paths are recorded, not the secret values |
| Provider settings | HA/failover enabled, private network access enabled, auth/TLS configured, persistence or scheduled snapshots enabled, retention and documented RPO/RTO accepted |
| Observability | Grafana dashboards `MPP Redis SLO Baseline` and `MPP Redis Error Budget (App-Side)`, Redis exporter, provider metrics, provider event log, and application logs available |
| Rollback owner | Named owner with access to apply `deploy/kubernetes/overlays/production-self-hosted-ha` and to restore Redis snapshots if required |
| Artifacts directory | Local path such as `artifacts/redis-managed-cutover/<run-id>` for inventory, copy, TTL diff, and monitoring evidence |
| Cutover record | `doc/managed-redis-production-cutover-record.md` copied or updated with real issue #338 production evidence |

Set the shell context for every command block:

```bash
export MPP_APP_NS=mpp-system
export MPP_OBS_NS=mpp-observability
export SOURCE_OVERLAY=deploy/kubernetes/overlays/production-self-hosted-ha
export TARGET_OVERLAY=deploy/kubernetes/overlays/production-managed
export RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
export ARTIFACT_DIR="artifacts/redis-managed-cutover/${RUN_ID}"
mkdir -p "$ARTIFACT_DIR"
```

## Production Cutover Record

Use `doc/managed-redis-production-cutover-record.md` as the execution record
for issue #338. Keep it redacted, attach artifact paths or dashboard links, and
do not mark the issue complete until prechecks, data movement, TTL diff,
endpoint switch, soak monitoring, and rollback readiness are all accepted with
real production evidence.

## Dry Run Record

Complete one dry run against non-production managed Redis or an isolated
provider restore target before scheduling production.

| Field | Value |
| --- | --- |
| Dry run environment | Pending |
| Source endpoint | Pending |
| Managed target endpoint | Pending |
| Operator | Pending |
| Window start and end | Pending |
| Precheck artifact path | Pending |
| Data copy artifact path | Pending |
| TTL diff artifact path | Pending |
| Endpoint switch artifact path | Pending |
| Monitoring result | Pending |
| Rollback exercise result | Pending |
| Accepted gaps | Pending |

The dry run is complete only when the same precheck, optional freeze, data copy,
TTL diff, endpoint switch, monitoring, and rollback commands have been rehearsed
or deliberately marked not applicable by the change owner. A local static docs
review does not satisfy this gate.

Current issue #337 isolated dry-run evidence:

| Check | Result |
| --- | --- |
| Migration rehearsal copy and TTL diff harness | `ruby script/kubernetes/test_redis_ha_migration_rehearsal.rb` passed with 7 runs and 39 assertions on 2026-06-19 |
| Keyspace inventory and managed TLS option harness | `ruby script/redis/test_keyspace_inventory.rb` passed with 8 runs and 322 assertions on 2026-06-19 |
| Scope | Isolated local test harness only; production execution still requires accepted provider validation evidence from `doc/managed-redis-nonprod-validation.md` |

## 1. Prechecks

Confirm the active cluster and branch context:

```bash
kubectl config current-context
kubectl get ns "$MPP_APP_NS" "$MPP_OBS_NS"
git status --short
git rev-parse HEAD
```

Confirm the source self-hosted HA Redis endpoint is the active production
endpoint:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config \
  -o jsonpath='{.data.APP_ENV}{" REDIS_ENDPOINT_MODE="}{.data.REDIS_ENDPOINT_MODE}{" REDIS_SENTINEL_ADDRS="}{.data.REDIS_SENTINEL_ADDRS}{" REDIS_SENTINEL_MASTER_NAME="}{.data.REDIS_SENTINEL_MASTER_NAME}{" REDIS_ADDR="}{.data.REDIS_ADDR}{" REDIS_TLS="}{.data.REDIS_TLS}{"\n"}' \
  | tee "$ARTIFACT_DIR/source-active-config.txt"
```

Expected source values are `APP_ENV=production`,
`REDIS_ENDPOINT_MODE=sentinel`, `REDIS_SENTINEL_ADDRS=redis-ha-sentinel:26379`,
`REDIS_SENTINEL_MASTER_NAME=mpp-redis-ha`, and `REDIS_TLS=false`.

Check source HA health:

```bash
kubectl rollout status statefulset/redis-ha-primary -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status statefulset/redis-ha-replica -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status statefulset/redis-ha-sentinel -n "$MPP_APP_NS" --timeout=5m

kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-primary -c redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
  fi
  redis-cli --raw --no-auth-warning PING
  redis-cli --raw --no-auth-warning INFO replication | tr -d "\r" | grep -E "role:|connected_slaves:"
' | tee "$ARTIFACT_DIR/source-ha-health.txt"

kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-sentinel -c sentinel -- sh -ec '
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mpp-redis-ha
  redis-cli -p 26379 SENTINEL ckquorum mpp-redis-ha
' | tee "$ARTIFACT_DIR/source-sentinel-health.txt"
```

Confirm the managed provider endpoint and secret wiring without exposing secret
values:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config \
  -o jsonpath='{.data.REDIS_ADDR}{" REDIS_TLS="}{.data.REDIS_TLS}{" REDIS_DB="}{.data.REDIS_DB}{" REDIS_TLS_CA_FILE="}{.data.REDIS_TLS_CA_FILE}{" REDIS_TLS_SERVER_NAME="}{.data.REDIS_TLS_SERVER_NAME}{"\n"}' \
  | tee "$ARTIFACT_DIR/target-config-redacted.txt"

kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets \
  -o jsonpath='{.data.REDIS_PASSWORD}' | wc -c \
  | tee "$ARTIFACT_DIR/redis-password-base64-byte-count.txt"
```

Render and validate the managed production overlay after all placeholders,
provider hostnames, image tags, External Secrets paths, TLS settings, and Redis
persistence decisions are patched:

```bash
rendered="$(mktemp)"
kubectl kustomize "$TARGET_OVERLAY" > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb "$TARGET_OVERLAY" "$rendered"
MPP_KUBERNETES_VALIDATE_DEPLOYABLE=1 \
  ruby script/kubernetes/validate-rendered-manifests.rb "$TARGET_OVERLAY" "$rendered"
cp "$rendered" "$ARTIFACT_DIR/production-managed-rendered.yaml"
```

Run a read-only source keyspace inventory:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-primary -c redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
  fi
  redis-cli --raw --no-auth-warning INFO keyspace
  redis-cli --raw --no-auth-warning DBSIZE
' | tee "$ARTIFACT_DIR/source-keyspace-summary.txt"

kubectl port-forward -n "$MPP_APP_NS" statefulset/redis-ha-primary 6379:6379 \
  > "$ARTIFACT_DIR/source-redis-port-forward.log" 2>&1 &
source_pf_pid="$!"
trap 'kill "$source_pf_pid" 2>/dev/null || true' EXIT
sleep 3

REDIS_PASSWORD="$(kubectl get secret -n "$MPP_APP_NS" mpp-app-secrets -o jsonpath='{.data.REDIS_PASSWORD}' | base64 --decode)"
REDIS_ADDR=127.0.0.1:6379 \
REDIS_PASSWORD="$REDIS_PASSWORD" \
ruby script/redis/keyspace_inventory.rb \
  --redis-db "${REDIS_DB:-0}" \
  --max-keys 100000 \
  > "$ARTIFACT_DIR/source-keyspace-inventory.json"
unset REDIS_PASSWORD
kill "$source_pf_pid" 2>/dev/null || true
trap - EXIT
```

Confirm dashboards and alerts before proceeding:

```promql
max(redis_up{service="redis"})
sum(rate(mpp_redis_operations_total{status="error"}[5m])) or vector(0)
sum(rate(mpp_redis_fallback_total[5m])) or vector(0)
sum(rate(mpp_redis_cache_misses_total[5m])) or vector(0)
```

Stop the cutover if any of these prechecks fail, if the managed provider
snapshot policy is not active, or if the non-production validation report still
has pending production decisions.

## 2. Optional Write Freeze

Use a write freeze when source Redis contains non-idempotent queue or
coordination state that cannot tolerate copy-time drift. At minimum, freeze when
the source inventory shows active `asynq:*`, `mpp:publish:lock:*`, browser
session coordination keys, or a high write rate during the planned window.

MPP does not currently expose a single app-wide maintenance flag. Use the
smallest operational freeze that fits the risk:

```bash
kubectl scale deployment/publish-worker -n "$MPP_APP_NS" --replicas=0
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
```

If the change owner requires a hard write stop, apply the environment's ingress
or gateway control to reject user write traffic while leaving health checks and
rollback access available. Record the exact gateway change in the change record.

If a full freeze is not used, record the accepted drift risk and keep the copy
and endpoint switch window short. Short-TTL keys may expire between the copy and
TTL diff; this is acceptable only when the owner signs off using the
responsibility tiers in `doc/redis-dependency-map.md`.

## 3. Data Copy

Take or confirm a source self-hosted HA Redis backup before copying:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-primary -c redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
  fi
  redis-cli --raw --no-auth-warning SAVE
  redis-cli --raw --no-auth-warning LASTSAVE
' | tee "$ARTIFACT_DIR/source-lastsave.txt"
```

Use the managed provider's supported online import, migration service, or backup
restore workflow as the primary data-copy mechanism when available. It must
preserve Redis data types and TTLs and produce an import job identifier.

Record:

| Field | Value |
| --- | --- |
| Copy mechanism | Pending |
| Provider import job or snapshot ID | Pending |
| Source backup or snapshot ID | Pending |
| Copy start timestamp | Pending |
| Copy completion timestamp | Pending |
| Keys copied | Pending |
| Copy warnings | Pending |

When the provider cannot import directly from self-hosted HA Redis, run a
controlled copy from a trusted migration host in the private network using a
tool that supports TLS/auth on the managed target and preserves TTLs. Do not use
ad hoc `KEYS`, `GET`, or string-only shell loops. The previous self-hosted HA
`MIGRATE COPY REPLACE` rehearsal in
`script/kubernetes/redis-ha-migration-rehearsal.rb` is acceptable for
self-hosted-to-self-hosted rehearsals only; use provider tooling or a
TLS-capable migration tool for managed Redis.

After the copy, keep the self-hosted HA Redis source running. Do not delete or
flush source keys during the soak window.

## 4. TTL Diff

Collect a target inventory from a migration host that has `redis-cli` access to
the managed endpoint. Use TLS and CA/SNI options that match the provider
settings:

```bash
REDIS_ADDR="<managed-redis-host>:<port>" \
REDIS_DB="<db-index>" \
REDIS_TLS="<true-or-false>" \
REDIS_TLS_CA_FILE="<provider-ca-file-if-required>" \
REDIS_TLS_SERVER_NAME="<provider-sni-if-required>" \
ruby script/redis/keyspace_inventory.rb \
  --max-keys 100000 \
  > "$ARTIFACT_DIR/target-keyspace-inventory.json"
```

Compare source and target inventories. The review must include:

- total key count and memory delta;
- missing `R0` and `R1` patterns;
- `asynq:*` queue key presence when publish/email/read-model work is expected;
- TTL min/max movement by pattern;
- keys without expire that unexpectedly gained or lost TTL;
- provider command visibility gaps such as blocked `MEMORY USAGE`.

Record the result:

| Diff check | Accepted result |
| --- | --- |
| Source key count | Pending |
| Target key count | Pending |
| Missing critical patterns | Pending |
| TTL drift summary | Pending |
| Unexpected no-expire changes | Pending |
| Provider command gaps | Pending |
| Owner approval | Pending |

Stop before endpoint switch if any `R0` or `R1` pattern is missing without an
approved recovery plan, if Asynq state is unexpectedly absent, or if provider
restore/import warnings indicate partial data.

## 5. Endpoint Switch

Patch the production managed overlay or provider-specific overlay so application
Pods select the managed endpoint:

```yaml
REDIS_ENDPOINT_MODE: direct
REDIS_ADDR: <managed-redis-host>:<port>
REDIS_DB: "0"
REDIS_TLS: "true"
REDIS_TLS_CA_FILE: <mounted-provider-ca-file-if-required>
REDIS_TLS_SERVER_NAME: <provider-sni-if-required>
REDIS_SENTINEL_ADDRS: ""
REDIS_SENTINEL_MASTER_NAME: mpp-redis-ha
```

Keep `REDIS_PASSWORD` materialized through External Secrets when the provider
requires auth. Keep the `redis` ExternalName patched to the provider hostname
only when the environment needs that stable in-cluster alias; `REDIS_ADDR` must
remain the provider hostname when TLS hostname verification is enabled.

Apply the overlay and restart Redis-dependent workloads:

```bash
kubectl apply -k "$TARGET_OVERLAY"
kubectl rollout restart deployment/backend deployment/publish-worker \
  deployment/browser-worker deployment/collab-service -n "$MPP_APP_NS"

kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/browser-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/collab-service -n "$MPP_APP_NS" --timeout=5m
```

Confirm running config without printing secrets:

```bash
kubectl get configmap -n "$MPP_APP_NS" mpp-app-config \
  -o jsonpath='{.data.APP_ENV}{" REDIS_ENDPOINT_MODE="}{.data.REDIS_ENDPOINT_MODE}{" REDIS_ADDR="}{.data.REDIS_ADDR}{" REDIS_TLS="}{.data.REDIS_TLS}{" REDIS_TLS_CA_FILE="}{.data.REDIS_TLS_CA_FILE}{" REDIS_TLS_SERVER_NAME="}{.data.REDIS_TLS_SERVER_NAME}{"\n"}' \
  | tee "$ARTIFACT_DIR/post-switch-config.txt"
```

Run one Redis-backed application smoke check for each active workflow owner can
safely exercise:

```bash
kubectl port-forward -n "$MPP_APP_NS" service/backend 8080:8080 \
  > "$ARTIFACT_DIR/backend-port-forward.log" 2>&1 &
backend_pf_pid="$!"
trap 'kill "$backend_pf_pid" 2>/dev/null || true' EXIT
sleep 3
curl -fsS http://127.0.0.1:8080/ready | tee "$ARTIFACT_DIR/backend-ready.json"
kill "$backend_pf_pid" 2>/dev/null || true
trap - EXIT

kubectl port-forward -n "$MPP_APP_NS" service/browser-worker 8081:8081 \
  > "$ARTIFACT_DIR/browser-worker-port-forward.log" 2>&1 &
browser_worker_pf_pid="$!"
trap 'kill "$browser_worker_pf_pid" 2>/dev/null || true' EXIT
sleep 3
curl -fsS http://127.0.0.1:8081/ready | tee "$ARTIFACT_DIR/browser-worker-ready.json"
kill "$browser_worker_pf_pid" 2>/dev/null || true
trap - EXIT
```

If publish workers were frozen, scale them back after readiness and smoke checks
pass:

```bash
kubectl scale deployment/publish-worker -n "$MPP_APP_NS" --replicas=<previous-replica-count>
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
```

## 6. Monitoring And Soak

Watch the managed endpoint for at least the approved soak period before closing
the change. Use both provider metrics and MPP dashboards.

Required Prometheus checks:

```promql
max(redis_up{service="redis"})
max by (cmd, quantile) (redis_latency_percentiles_usec{service="redis",quantile=~"0.95|0.99"}) / 1000000
max(redis_memory_used_bytes{service="redis"})
(
  (max(redis_memory_max_bytes{service="redis"}) - max(redis_memory_used_bytes{service="redis"}))
  / max(redis_memory_max_bytes{service="redis"})
) and max(redis_memory_max_bytes{service="redis"}) > 0
max(redis_connected_clients{service="redis"})
sum(rate(redis_rejected_connections_total{service="redis"}[5m])) or vector(0)
sum(rate(redis_evicted_keys_total{service="redis"}[5m])) or vector(0)
sum(rate(mpp_redis_operations_total{status="error"}[5m])) or vector(0)
sum(rate(mpp_redis_fallback_total[5m])) or vector(0)
sum by (group, error_class) (rate(mpp_redis_errors_total[5m]))
(
  sum(rate(mpp_redis_cache_hits_total[5m]))
  /
  (sum(rate(mpp_redis_cache_hits_total[5m])) + sum(rate(mpp_redis_cache_misses_total[5m])))
) or vector(0)
```

Record:

| Signal | Result |
| --- | --- |
| Redis availability | Pending |
| Redis p95/p99 latency | Pending |
| Redis memory usage and headroom | Pending |
| Redis connection count | Pending |
| Rejected connections | Pending |
| Evictions | Pending |
| App Redis error rate | Pending |
| App fallback rate | Pending |
| Cache hit rate | Pending |
| Provider failover or maintenance events | Pending |
| User-facing incident reports | Pending |
| Rollback decision | Pending |

Keep self-hosted HA Redis and its PVCs intact until the Phase 4.5
decommissioning issue is complete and the rollback owner signs off.

## 7. Rollback

Roll back application traffic when any stop condition occurs during the window:

- backend, browser-worker, publish-worker, or collab-service readiness fails and
  does not recover inside the approved RTO;
- provider Redis rejects connections, fails auth/TLS, or reports failover loops;
- `mpp_redis_errors_total`, `mpp_redis_fallback_total`, Redis latency, or user
  incidents exceed the change record threshold;
- TTL diff shows missing critical state after switch;
- provider import produced partial data or corruption warnings.

Switch traffic back to self-hosted HA Redis by applying the previous production
self-hosted HA overlay or by patching the live ConfigMap while preparing the
overlay rollback:

```bash
kubectl apply -k "$SOURCE_OVERLAY"
kubectl patch configmap mpp-app-config -n "$MPP_APP_NS" --type merge -p \
  '{"data":{"REDIS_ENDPOINT_MODE":"sentinel","REDIS_SENTINEL_ADDRS":"redis-ha-sentinel:26379","REDIS_SENTINEL_MASTER_NAME":"mpp-redis-ha","REDIS_ADDR":"redis:6379","REDIS_TLS":"false","REDIS_TLS_CA_FILE":"","REDIS_TLS_CA_CERT":"","REDIS_TLS_SERVER_NAME":""}}'

kubectl rollout restart deployment/backend deployment/publish-worker \
  deployment/browser-worker deployment/collab-service -n "$MPP_APP_NS"

kubectl rollout status deployment/backend -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/publish-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/browser-worker -n "$MPP_APP_NS" --timeout=5m
kubectl rollout status deployment/collab-service -n "$MPP_APP_NS" --timeout=5m
```

Run post-rollback health checks:

```bash
kubectl exec -n "$MPP_APP_NS" statefulset/redis-ha-primary -c redis -- sh -ec '
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    export REDISCLI_AUTH="$REDIS_PASSWORD"
  fi
  redis-cli --raw --no-auth-warning PING
  redis-cli --raw --no-auth-warning INFO keyspace
'
kubectl logs -n "$MPP_APP_NS" deployment/backend --tail=100
kubectl logs -n "$MPP_APP_NS" deployment/publish-worker --tail=100
```

Snapshot restore is required only when the self-hosted HA source accepted writes
that were later corrupted, flushed, deleted, or allowed to expire beyond the
accepted RPO while traffic was away from it. If the source remained untouched,
rollback is an endpoint switch and workload restart only. If restore is
required, restore the last approved self-hosted HA Redis snapshot into the
self-hosted HA primary, confirm `INFO keyspace` and sampled critical keys, then
restart the Redis-dependent workloads.

Do not copy data back from managed Redis during emergency rollback unless the
incident commander explicitly accepts the risk. A reverse copy can reintroduce
the bad state that triggered rollback.

## 8. Closeout

Attach these artifacts to the change record:

```text
Runbook issue: #337
Cutover issue: #338
Run ID:
Cutover record:
Production dry run evidence:
Non-production validation report:
Managed provider and plan:
Overlay path and Git SHA:
Operator:
Rollback owner:
Window start:
Window end:
Source backup or snapshot:
Copy mechanism and job ID:
Source inventory:
Target inventory:
TTL diff approval:
Endpoint switch timestamp:
Monitoring dashboard links:
Rollback decision:
Follow-up issues:
```

After a successful soak, open the Phase 4.5 decommissioning work. Until that
work is complete, treat self-hosted HA Redis as the rollback system of record
and keep backups, PVCs, and overlay manifests available.
