# Managed Redis Production Cutover Record

Use this record for the Phase 4.4 production cutover in issue #338. It is the
auditable companion to `doc/managed-redis-production-migration-runbook.md`.

Do not mark issue #338 complete until every completion gate is accepted with
real production evidence from the approved cutover window. Do not paste Redis
passwords, provider secrets, private keys, or raw credential material into this
record.

## Execution Summary

| Field | Value |
| --- | --- |
| Issue | #338 |
| Runbook | `doc/managed-redis-production-migration-runbook.md` |
| Execution status | Pending production execution |
| Run ID | Pending |
| Production window start | Pending |
| Production window end | Pending |
| Agreed soak period | Pending |
| Operator | Pending |
| Rollback owner | Pending |
| Managed Redis provider and plan | Pending |
| Managed endpoint | Pending |
| Overlay path and Git SHA | Pending |
| Artifact directory | Pending |
| Change record link | Pending |

## Completion Gates

| Gate | Evidence required | Status |
| --- | --- | --- |
| Prechecks complete | Source self-hosted HA Redis is healthy, managed endpoint config is validated, provider snapshots are enabled, dashboards are reachable, and rollback owner is present | Pending |
| Data movement complete | Provider import, migration job, or approved copy tool completed with source backup or snapshot ID recorded | Pending |
| TTL diff accepted | Source and target inventories show no unapproved missing `R0` or `R1` patterns, no unexpected no-expire changes, and owner approval for drift | Pending |
| Endpoint switch complete | Production workloads run with managed `REDIS_ADDR`, expected TLS/auth settings, and successful readiness smoke checks | Pending |
| Soak stable | Error rate, Redis latency, memory, connection count, and cache hit rate stay within the change record thresholds for the full agreed soak period | Pending |
| Rollback ready | Self-hosted HA Redis, PVCs, source overlay, source snapshot, and snapshot restore option remain available through the soak | Pending |
| Closeout accepted | Rollback decision is recorded and Phase 4.5 decommissioning remains blocked until rollback owner signs off | Pending |

## Evidence Index

| Artifact | Path or link | Notes |
| --- | --- | --- |
| Source active config | Pending | Redacted ConfigMap output only |
| Source HA health | Pending | Primary, replica, and Sentinel checks |
| Managed target config | Pending | Redacted endpoint, TLS, DB, CA, and SNI settings |
| Rendered production managed overlay | Pending | Rendered manifest from the exact Git SHA |
| Source keyspace inventory | Pending | `script/redis/keyspace_inventory.rb` output |
| Source backup or snapshot | Pending | Snapshot ID or storage backup reference |
| Provider import or copy job | Pending | Provider job ID or migration tool report |
| Target keyspace inventory | Pending | Managed endpoint inventory output |
| TTL diff approval | Pending | Owner-approved count and TTL review |
| Endpoint switch evidence | Pending | Apply, restart, rollout, and running config output |
| Smoke checks | Pending | Backend and browser-worker readiness checks |
| Soak monitoring export | Pending | Dashboard snapshot, Prometheus export, or incident record |
| Rollback readiness confirmation | Pending | Self-hosted HA overlay, PVC, and restore confirmation |

## Precheck Record

| Check | Expected result | Evidence |
| --- | --- | --- |
| Production context confirmed | Operator confirms the active production Kubernetes context | Pending |
| Source endpoint active before switch | `APP_ENV=production`, `REDIS_ENDPOINT_MODE=sentinel`, `REDIS_SENTINEL_ADDRS=redis-ha-sentinel:26379`, `REDIS_SENTINEL_MASTER_NAME=mpp-redis-ha`, `REDIS_TLS=false` | Pending |
| Source HA healthy | Primary, replicas, and Sentinel report healthy status and quorum | Pending |
| Managed endpoint validated | Provider hostname, port, TLS, CA, SNI, DB index, auth, and private-network access are accepted | Pending |
| Provider persistence enabled | Snapshot or persistence policy, retention, RPO, and restore procedure are accepted | Pending |
| Production managed overlay validated | Static and deployable manifest validation pass for the exact overlay and Git SHA | Pending |
| Observability available | Redis SLO dashboard, Redis error-budget dashboard, Redis exporter, provider metrics, provider events, and app logs are available | Pending |
| Rollback owner ready | Owner can apply the self-hosted HA overlay before issue #339, or use the issue #339 historical chart and snapshot after decommission | Pending |

## Data Movement And TTL Diff

| Field | Value |
| --- | --- |
| Write freeze used | Pending |
| Accepted drift risk if no freeze | Pending |
| Source backup or snapshot ID | Pending |
| Copy mechanism | Pending |
| Provider import or migration job ID | Pending |
| Copy start timestamp | Pending |
| Copy completion timestamp | Pending |
| Source key count | Pending |
| Target key count | Pending |
| Missing critical patterns | Pending |
| TTL drift summary | Pending |
| Unexpected no-expire changes | Pending |
| Provider command gaps | Pending |
| Owner approval | Pending |

## Endpoint Switch Record

| Check | Expected result | Evidence |
| --- | --- | --- |
| Managed overlay applied | Production app config selects managed Redis in direct mode | Pending |
| Redis-dependent workloads restarted | `backend`, `publish-worker`, `browser-worker`, and `collab-service` roll out successfully | Pending |
| Running config confirmed | Redacted ConfigMap output shows managed `REDIS_ADDR`, `REDIS_TLS`, CA, and SNI settings | Pending |
| Backend readiness passes | `/ready` succeeds after the switch | Pending |
| Browser-worker readiness passes | `/ready` succeeds after the switch | Pending |
| Frozen workers restored | Publish workers return to the previous replica count when a freeze was used | Pending |

## Soak Monitoring Record

Record baseline, immediate post-switch, and end-of-soak values. Attach a
dashboard snapshot, Prometheus export, or incident timeline for each accepted
value.

| Signal | Query or source | Baseline | Post-switch | End of soak | Accepted |
| --- | --- | --- | --- | --- | --- |
| Redis availability | `max(redis_up{service="redis"})` | Pending | Pending | Pending | Pending |
| Redis p95/p99 latency | `max by (cmd, quantile) (redis_latency_percentiles_usec{service="redis",quantile=~"0.95\|0.99"}) / 1000000` | Pending | Pending | Pending | Pending |
| Redis memory usage | `max(redis_memory_used_bytes{service="redis"})` and memory headroom | Pending | Pending | Pending | Pending |
| Redis connection count | `max(redis_connected_clients{service="redis"})` | Pending | Pending | Pending | Pending |
| Rejected connections or commands | `sum(rate(redis_rejected_connections_total{service="redis"}[5m])) or vector(0)` plus command rejection metrics when available | Pending | Pending | Pending | Pending |
| Evictions | `sum(rate(redis_evicted_keys_total{service="redis"}[5m])) or vector(0)` | Pending | Pending | Pending | Pending |
| App Redis error rate | `sum(rate(mpp_redis_operations_total{status="error"}[5m])) or vector(0)` | Pending | Pending | Pending | Pending |
| App fallback rate | `sum(rate(mpp_redis_fallback_total[5m])) or vector(0)` | Pending | Pending | Pending | Pending |
| Cache hit rate | `sum(rate(mpp_redis_cache_hits_total[5m])) / (sum(rate(mpp_redis_cache_hits_total[5m])) + sum(rate(mpp_redis_cache_misses_total[5m])))` | Pending | Pending | Pending | Pending |
| Provider events | Provider metrics and event log | Pending | Pending | Pending | Pending |
| User-facing incidents | Incident tracker and support channels | Pending | Pending | Pending | Pending |

## Rollback Readiness Record

| Check | Expected result | Evidence |
| --- | --- | --- |
| Self-hosted HA Redis retained | Source StatefulSets, Services, and PVCs remain intact through the soak | Pending |
| Source overlay available | Before issue #339, `deploy/kubernetes/overlays/production-self-hosted-ha` renders and remains applicable; after issue #339, use `doc/self-hosted-redis-decommission-record.md` for the historical Git SHA and snapshot rollback path | Pending |
| Source data protected | Source backup or snapshot is retained and restore path is known | Pending |
| Snapshot restore option confirmed | Restore target, owner, and expected RPO/RTO are recorded | Pending |
| Emergency endpoint rollback tested or rehearsed | Operator can switch traffic back to Sentinel mode and restart Redis-dependent workloads | Pending |
| Reverse copy decision recorded | Incident commander approval is required before any managed-to-self-hosted reverse copy | Pending |

## Closeout Decision

| Decision | Value |
| --- | --- |
| Production stable for agreed soak period | Pending |
| Critical Redis-related error spike observed | Pending |
| Rollback executed | Pending |
| Rollback remains available after closeout | Pending |
| Phase 4.5 decommissioning issue | Pending |
| Follow-up issues | Pending |
| Final owner sign-off | Pending |
