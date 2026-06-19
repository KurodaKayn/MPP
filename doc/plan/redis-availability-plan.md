# MPP Redis Availability Evolution Plan

## 0. Execution Status

This document is the entry point for the Redis availability workstream. The goal is to improve Redis availability, recoverability, observability, and migration capability step by step, without introducing infrastructure that is too heavy for the current stage.

Phase 1-2 target: pragmatic high availability. After a single Redis Pod, node, or short network failure, core services should recover within 1-5 minutes. A small amount of short-TTL transient state loss is acceptable.

Production final target: Redis Cluster. The final state must support multiple shards, multiple replicas, automatic failover, TLS/auth, backup and restore, maintenance windows, and clear SLA ownership. Prefer a provider-backed managed Redis Cluster. If managed Redis is unavailable, use a mature chart/operator to self-host Redis Cluster.

Current overall progress: about `69%`.

| Phase | Weight | Current Completion | Status | Done | Next |
| --- | ---: | ---: | --- | --- | --- |
| Phase 0: responsibility and risk baseline | 10% | 100% | Done | Inventory script, responsibility labels, Redis SLO baseline, and [dependency map](../redis-dependency-map.md) added | Start Phase 1 single-instance hardening |
| Phase 1: single-instance hardening | 15% | 100% | Done | Current single-instance deployment direction clear; Redis persistence baseline, probes, resources, graceful termination, backup baseline, restore runbook, runtime config hardening, and capacity guardrail alerts added | Maintain as HA fallback baseline |
| Phase 2: self-hosted HA | 20% | 100% | Done | HA deployment, endpoint abstraction, failover validation, migration rehearsal, and production HA cutover completed | Use HA setup as the baseline for managed Redis validation |
| Phase 3: app-side fault tolerance | 20% | 100% | Done | Role-specific Redis timeout/retry baselines, degraded cache modes, cache stampede protection, lock safety hardening, and Redis error-budget reporting added | Use app-side metrics during HA failover validation and operational drills |
| Phase 4: production managed Redis HA | 15% | 25% | In Progress | Provider endpoint parameterization is complete; [managed Redis non-production validation report](../managed-redis-nonprod-validation.md) added for latency, failover, restore, TLS/auth, metrics, teardown, and risk evidence | Run the selected provider validation and record observed evidence before migration runbook work |
| Phase 5: Redis Cluster target state | 15% | 0% | Not Started | Final target confirmed | Design key model, client compatibility, and cutover path |
| Phase 6: drills and operations loop | 5% | 0% | Not Started | Not yet started | Add periodic failover, restore, and capacity drills |

## 1. Background And Problem

Redis is currently too important in the system. It likely carries responsibilities such as cache, sessions, rate limits, distributed locks, queues, deduplication, idempotency, and real-time state. If Redis is unavailable, slow, or loses data, multiple services may be affected at the same time.

Introducing Redis Cluster directly is not the first engineering step. Before Cluster, the system must first clarify Redis responsibilities, split criticality, harden single-instance operations, add self-hosted high availability, add app-side fault tolerance, and make future migration low-risk.

This plan is intentionally gradual. Each subsection maps to one PR. Each PR must be independently mergeable, independently rollbackable, and verifiable. Do not mix Redis HA, Cluster sharding, app fault tolerance, and managed migration in one PR.

## 2. Goals And Non-Goals

### Goals

| Goal | Description |
| --- | --- |
| Clarify Redis responsibilities | Identify which keys and use cases Redis carries, and whether each one can be lost, rebuilt, or delayed. |
| Improve availability gradually | Move from single instance to local HA, then managed HA, then Redis Cluster target state. |
| Reduce blast radius | Prevent Redis failure from taking down unrelated business flows. |
| Improve observability | Make latency, error rate, memory, eviction, replication, and failover visible. |
| Improve recoverability | Ensure backups, restore drills, and rollback paths exist before high-risk migration. |
| Prepare for Redis Cluster | Adjust key naming, client usage, and multi-key operations in advance. |

### Non-Goals

| Item | Reason |
| --- | --- |
| One-shot Redis Cluster rollout | Too large a blast radius; requires client, key, ops, and migration readiness. |
| Replacing all Redis usage | This plan improves Redis safety; it does not redesign all dependent product logic. |
| Introducing a separate message queue in the same PR | Queue decoupling can be separate work after Redis responsibility inventory. |
| Guaranteeing zero Redis data loss for all key types | Some transient keys can accept loss; only critical responsibilities require stronger guarantees. |

## 3. Redis Responsibility Tiers

Every Redis usage must be classified before deep infrastructure change.

| Tier | Typical Usage | Data Loss Tolerance | Availability Requirement | Recommended Direction |
| --- | --- | --- | --- | --- |
| R0 Critical coordination | Distributed locks, idempotency, rate-limit counters affecting money or permissions | Very low | Must fail closed or safely degrade | Add strict TTL, fencing token, DB fallback, audit logs |
| R1 User continuity | Session, login state, temporary user workflow state | Low | Short outage acceptable; long outage not acceptable | HA Redis, fallback UX, clear re-login behavior |
| R2 Performance cache | DB/API cache, computed result cache | Medium | Cache miss must not overload origin | Cache-aside, stampede protection, origin rate control |
| R3 Ephemeral signal | Presence, online status, short-lived deduplication | High | Can degrade or be delayed | Allow drop, rebuild, or delayed refresh |
| R4 Queue-like usage | Delayed jobs, event buffer, stream/list queue | Depends | Must not silently lose business-critical jobs | Move critical queues out of Redis or use durable Redis Streams with explicit policy |

## 4. Phase 0: Redis Responsibility And Risk Baseline

This phase does not change Redis infrastructure. It builds facts first.

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 0.1: Build Redis keyspace inventory | Know what Redis is used for | Add script or internal command to scan key patterns, TTL, memory usage, type, and owner | Inventory output includes key pattern, owner, TTL, type, memory, read/write service | Remove script or disable command | No Redis topology change |
| PR 0.2: Define Redis responsibility labels | Classify risk | Add labels `R0-R4` in docs/config comments; map key patterns to labels | Every known key pattern has a tier and owner | Revert documentation/config metadata | No code behavior change |
| PR 0.3: Add Redis SLO and alert baseline | Make Redis risk visible | Add metrics dashboard and alerts: availability, p95/p99 latency, connection errors, memory, evictions, blocked clients | Dashboard visible; alerts tested in non-prod | Remove alerts or lower severity | No failover or scaling change |
| PR 0.4: Add Redis dependency map | Know blast radius | Add [Redis dependency map](../redis-dependency-map.md) documenting which services depend on Redis and which requests fail when Redis fails | Each service has Redis dependency and degradation behavior recorded | Revert document | No application fallback implementation |

## 5. Phase 1: Single-Instance Reliability Hardening

This phase keeps Redis single-instance, but makes it less fragile and easier to restore.

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 1.1: Add Redis persistence baseline | Avoid unnecessary data loss on restart | Enable PVC-backed `/data`, version `redis-persistence-config`, choose RDB/AOF policy by environment, document persistence mode | Redis restart keeps expected data class; ephemeral keys may expire normally | Disable persistence or revert volume config | No HA |
| PR 1.2: Add probes and resource limits | Reduce unhealthy Pod behavior | Add readiness/liveness probes, CPU/memory requests/limits, graceful termination | Pod fails readiness when Redis cannot serve; restart behavior verified | Revert deployment values | No topology change |
| PR 1.3: Add backup and restore runbook | Make restore possible | Add scheduled backup for persistence files or managed snapshot equivalent; write restore steps | Restore tested in non-prod with recorded RTO/RPO | Disable schedule; keep last backup | No production cutover |
| PR 1.4: Add Redis config hardening | Reduce self-inflicted outages | Tune `maxmemory-policy`, `timeout`, `tcp-keepalive`, `appendonly`, slowlog, auth if applicable | Config is versioned; memory pressure behavior known | Revert values | No Cluster-specific config |
| PR 1.5: Add capacity guardrails | Prevent silent memory exhaustion | Add memory headroom alert, eviction alert, connection count alert, command latency alert | Alert fires in controlled test | Disable noisy alert | No autoscaling |

## 6. Phase 2: Self-Hosted High Availability

This phase gives the current environment an HA Redis path without waiting for production managed Redis.

Recommended Phase 2 target: Redis replication plus Sentinel or an equivalent mature HA chart/operator. This is enough for early HA: one primary, one or more replicas, automatic failover, stable service discovery, and app reconnection.

### 6.1 HA Design Choice

| Option | Benefits | Costs | Applies To |
| --- | --- | --- | --- |
| Redis primary-replica + Sentinel | Mature; easy to understand; supports automatic failover; lower complexity than Cluster | Does not shard; client and service discovery must support failover | Phase 2 preferred |
| Redis operator | More automated lifecycle and failover | Adds operator dependency and CRD learning cost | Use if team already accepts operator model |
| Redis Cluster now | Final topology direction | Requires key-slot compatibility, client changes, and migration complexity | Reserve for Phase 5 |

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 2.1: Add HA Redis deployment in non-prod | Deploy HA topology safely | Add chart/values for primary, replica, Sentinel; keep old Redis running | Non-prod HA Redis starts and reports healthy replication | Delete HA release and point back to old Redis | No app traffic cutover |
| PR 2.2: Add Redis endpoint abstraction | Make topology swappable | Move Redis host/port/auth/db into config; separate logical clients by responsibility if needed | App can point to old or HA Redis only by config change | Revert config layer | No business logic rewrite |
| PR 2.3: Validate client failover behavior | Ensure app reconnects after failover | Add integration test or script that kills primary and verifies reconnect/write recovery | App recovers within target window after primary kill | Disable HA endpoint and use old Redis | No production migration |
| PR 2.4: Non-prod data migration rehearsal | Test moving data | Add migration script/runbook: snapshot, restore, key TTL validation, sampled diff | Rehearsal produces data count and TTL diff report | Discard HA Redis data | No production data movement |
| PR 2.5: Production HA cutover | Move production to self-hosted HA | Freeze risky writes if needed; migrate data; switch endpoint; monitor | Cutover succeeds; failover drill passes; no critical error spike | Switch endpoint back to old Redis; restore previous snapshot if required | No Redis Cluster |

## 7. Phase 3: App-Side Fault Tolerance And Recovery

Redis HA alone is not enough. Applications must behave correctly during slow Redis, failover, partial outage, and cache cold start.

Current Phase 3 status: done. Redis clients now use responsibility-aware timeout and retry profiles; cache paths have degraded modes, workload-labeled guard metrics, TTL jitter, and singleflight protection; R0 publish and browser-session lock paths have ownership checks and stale-owner recovery; and Redis error-budget metrics are exposed through the backend observability suite with a Grafana dashboard.

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 3.1: Standardize Redis timeout and retry | Prevent request pileups | Define connect timeout, command timeout, retry count, backoff, jitter by client type | Redis slowdown does not exhaust app workers in test | Revert client options | No topology change |
| PR 3.2: Add circuit breaker and degraded modes | Prevent Redis outage from becoming full app outage | For R2/R3 keys, allow cache bypass, stale read, or degraded response; for R0 fail safely | Simulated Redis outage returns controlled errors or degraded responses | Disable breaker flags | No new infrastructure |
| PR 3.3: Add cache stampede protection | Avoid origin overload after Redis restart | Add singleflight/lock, jittered TTL, early refresh, or rate limit around hot keys | Cache flush test does not overload DB/API | Disable protection per key group | No HA change |
| PR 3.4: Audit distributed lock usage | Prevent unsafe locks after failover | Add TTL, owner token, fencing token where needed; remove lock patterns without safe release | R0 lock sites have documented safety behavior | Revert individual lock change | No broad business refactor |
| PR 3.5: Add Redis error budget reporting | Tie app health to Redis | Add Redis degrade observer hooks, workload-aware operation helpers on cache and rate-limit paths, Prometheus metrics for operations/errors/fallbacks/cache hit-miss/breaker state, runtime wiring, and a Grafana dashboard | `/metrics` exposes `mpp_redis_*`; dashboard shows app impact during Redis test | Remove observer wiring, metrics, and dashboard | No infra migration |

## 8. Phase 4: Production Managed Redis HA Intermediate State

This phase moves production from self-managed Redis HA to managed Redis HA. It is not the final topology. It reduces operational burden before Redis Cluster.

### 8.1 Managed HA Selection Criteria

| Criterion | Requirement |
| --- | --- |
| Automatic failover | Must support primary failover without manual Pod operations |
| Backup and restore | Must support scheduled snapshots and point-in-time or documented restore path |
| Maintenance window | Must support controlled maintenance and version upgrades |
| Network security | Must support private network access, auth, and preferably TLS |
| Metrics and logs | Must expose latency, commands, memory, evictions, connections, replication, failover events |
| Scaling path | Must have a clear path from HA instance to Redis Cluster or managed Cluster |

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 4.1: Parameterize provider Redis endpoint | Make managed migration config-only | Add env/config support for managed endpoint, TLS, auth, database index, CA if needed | Non-prod app connects to managed Redis by config only | Point config back to self-hosted HA | No production cutover |
| PR 4.2: Managed Redis non-prod validation | Verify provider behavior | Use the [managed Redis non-production validation report](../managed-redis-nonprod-validation.md) to test latency, failover, backup restore, TLS/auth, metrics/log visibility, teardown safety, and provider limitations | Validation report includes observed RTO/RPO, latency, failover behavior, restore result, provider limitations, and required production settings | Tear down non-prod managed Redis | No production traffic |
| PR 4.3: Production migration runbook | Make cutover repeatable | Write exact steps: precheck, freeze if needed, data copy, TTL diff, switch, monitor, rollback | Runbook dry-run completed | Revert runbook | No infra change |
| PR 4.4: Production managed Redis cutover | Move production to managed HA | Execute runbook; monitor error rate, latency, memory, cache hit rate | Production stable for agreed soak period | Switch endpoint back to self-hosted HA; restore snapshot if required | No Cluster sharding |
| PR 4.5: Decommission old self-hosted Redis | Remove duplicate ops burden | After soak, stop old Redis writes, keep backup, remove old deployment | No traffic to old Redis; backup retained | Recreate from previous chart and snapshot | No app behavior change |

## 9. Phase 5: Redis Cluster Production Target State

Redis Cluster is the final target. Do not start this phase until key model and client compatibility are ready.

### 9.1 Prerequisites

| Prerequisite | Requirement |
| --- | --- |
| Client compatibility | All Redis clients support Cluster redirects (`MOVED`, `ASK`) and topology refresh |
| Key model | Multi-key operations use hash tags or are removed from cross-slot paths |
| Command audit | No unsupported Cluster commands remain on critical paths |
| Migration path | Data migration and dual-run/verification strategy defined |
| Rollback path | Previous managed HA Redis remains available until soak completes |
| Observability | Per-node, per-slot, failover, memory, and hot-key metrics are visible |

### 9.2 Cluster Readiness Work

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 5.1: Audit Cluster-incompatible Redis usage | Find blockers | Scan for multi-key commands, Lua scripts, transactions, DB index usage, blocking commands | Report lists each blocker with owner and fix path | Revert report only | No runtime change |
| PR 5.2: Add key hash-tag convention | Make related keys co-locate | Define key naming rules such as `{tenantId}` or `{userId}` for multi-key groups | New keys follow convention; lint/test catches violations | Revert lint rule or docs | No data migration |
| PR 5.3: Replace or constrain cross-slot operations | Remove Cluster blockers | Update `MGET`, `DEL`, pipelines, Lua, transactions to single-slot or split-safe alternatives | Tests pass against Redis Cluster in non-prod | Revert individual command changes | No production cutover |
| PR 5.4: Enable Cluster-capable clients | Make apps Cluster-ready | Configure clients for Cluster topology, redirect handling, retry, TLS/auth | App passes integration tests against non-prod Cluster | Point client back to standalone mode | No production traffic |
| PR 5.5: Deploy non-prod Redis Cluster | Validate final topology | Create managed/self-hosted Redis Cluster in non-prod; test failover, resharding, backup restore | Non-prod Cluster passes failover and restore tests | Destroy non-prod Cluster | No production change |
| PR 5.6: Rehearse data migration to Cluster | Prove migration path | Use provider migration tool, `redis-shake`, custom script, or dual-write depending on provider support | Rehearsal report includes count diff, TTL diff, hot-key check, latency | Discard target Cluster data | No production cutover |
| PR 5.7: Production Redis Cluster cutover | Move production to final topology | Execute migration, switch app endpoint/client mode, monitor soak metrics | Production stable; no Cluster redirect storms; latency acceptable | Switch back to managed HA Redis and restore if required | No unrelated feature changes |
| PR 5.8: Post-cutover resharding and cleanup | Stabilize final state | Tune shard count, memory distribution, hot keys, backup, alerts; remove old HA endpoint after soak | Slot distribution balanced; old Redis no longer receives traffic | Keep old Redis longer; revert cleanup | No product logic changes |

## 10. Phase 6: Continuous Drills And Operations Loop

Redis availability decays if drills stop. This phase turns Redis operations into a routine.

| PR | Goal | Main Changes | Acceptance | Rollback | Out Of Scope |
| --- | --- | --- | --- | --- | --- |
| PR 6.1: Add scheduled failover drill | Keep failover real | Define quarterly or monthly failover drill in non-prod, then production-safe window | Drill record includes RTO, errors, and fixes | Pause schedule | No topology change |
| PR 6.2: Add restore drill | Prove backups are usable | Restore snapshot to isolated Redis and run sampled validation | Restore RTO/RPO recorded | Pause schedule | No production restore |
| PR 6.3: Add capacity review | Prevent slow growth failure | Monthly memory, key count, hot key, command latency review | Review produces action items when thresholds exceeded | Pause review | No automatic scaling |
| PR 6.4: Add incident playbook | Make response repeatable | Document Redis latency, outage, memory, failover, data-loss response steps | On-call can follow playbook in simulation | Revert doc | No code change |

## 11. Recommended Execution Order

| Order | PR | Why First |
| ---: | --- | --- |
| 1 | PR 0.1 | No inventory means no safe topology decision |
| 2 | PR 0.2 | Responsibility tier decides persistence, fallback, and migration strictness |
| 3 | PR 0.3 | Need visibility before changing Redis |
| 4 | PR 1.1 | Persistence is baseline recoverability |
| 5 | PR 1.2 | Probes/resources reduce simple Pod-level failure |
| 6 | PR 1.3 | Backup and restore must exist before HA migration |
| 7 | PR 1.4 | Config hardening prevents known failure modes |
| 8 | PR 1.5 | Capacity guardrails catch growth risk |
| 9 | PR 2.1 | Build HA without traffic |
| 10 | PR 2.2 | Endpoint abstraction makes rollback possible |
| 11 | PR 2.3 | Failover must be proven before migration |
| 12 | PR 2.4 | Migration rehearsal reduces production risk |
| 13 | PR 2.5 | First HA production milestone |
| 14 | PR 3.1-3.5 | App-side resilience reduces future migration blast radius |
| 15 | PR 4.1-4.5 | Move operational burden to managed Redis HA |
| 16 | PR 5.1-5.8 | Reach Redis Cluster final state |
| 17 | PR 6.1-6.4 | Keep reliability from regressing |

## 12. Definition Of Done

| Area | Done Criteria |
| --- | --- |
| Redis responsibility | Every Redis key pattern has owner, tier, TTL policy, and recovery expectation |
| Single-instance baseline | Persistence, probes, resources, backup, restore, and alerts are in place |
| Self-hosted HA | Primary failure can recover within target window; app reconnect behavior verified |
| App resilience | Redis timeout, retry, circuit breaker, degraded mode, stampede protection, lock ownership safety, and error-budget reporting are implemented for critical paths |
| Managed HA | Production runs on managed Redis HA with backup, metrics, TLS/auth, and rollback path |
| Redis Cluster | Production runs on Redis Cluster; clients are Cluster-aware; cross-slot blockers resolved |
| Operations | Failover drill, restore drill, capacity review, and incident playbook are recurring |

Minimum acceptable milestone before calling Redis "reasonably available": Phase 0, Phase 1, Phase 2, and core Phase 3 PRs are complete.

Final target: Phase 5 complete. Production Redis runs on Redis Cluster.
