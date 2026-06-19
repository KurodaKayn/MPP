# MPP Redis Availability Notes

This document is the entry point for Redis reliability work in MPP. The project
has not been launched in production, so Redis documentation should describe
current deployable shapes, validation expectations, and design constraints. It
should not preserve historical production change records or tracker-specific
execution notes.

## Current Scope

Redis is used for transient runtime responsibilities: queues, locks, session
state, OAuth state, rate limits, cache entries, browser-session coordination,
and collaboration sync. PostgreSQL remains the durable source of truth for
business data.

The detailed Redis responsibility map lives in
`doc/redis/redis-dependency-map.md`. Cluster-readiness constraints live in
`doc/redis/redis-cluster-compatibility-audit.md`, and key naming rules live in
`doc/redis/redis-key-hash-tag-convention.md`.

## Operating Principles

| Principle | Guidance |
| --- | --- |
| Keep Redis responsibilities explicit | Every known Redis key pattern should have an owner, TTL policy, data-loss expectation, and recovery behavior. |
| Prefer provider-managed Redis for production-style Kubernetes overlays | Managed Redis keeps backup, failover, maintenance, and persistence ownership outside the app cluster. |
| Keep self-hosted Redis small and test-focused | Self-hosted manifests are useful for local, demo, and staging validation, but should not grow into a separate production operations burden. |
| Validate behavior before changing topology | Client reconnect behavior, persistence, failover, and degraded app behavior should be tested in non-production environments. |
| Treat Redis Cluster as a compatibility target, not an immediate default | Cluster requires client-mode changes, DB index constraints, hash-tagged keys, and removal of cross-slot command blockers. |

## Current Capabilities

| Area | Current state |
| --- | --- |
| Dependency inventory | Redis responsibilities are documented in the dependency map. |
| Keyspace inventory | `script/redis/keyspace_inventory.rb` scans standalone/Sentinel Redis key patterns, TTL, type, and memory samples. |
| Self-hosted baseline | The self-hosted Kubernetes package includes Redis persistence, probes, resource limits, backup hooks, and runtime hardening config. |
| Non-production HA validation | `deploy/kubernetes/data-services/redis-ha-nonprod` can run a parallel Sentinel topology for staging drills without changing the default app Redis endpoint. |
| Managed Redis config | Managed overlays parameterize Redis host, TLS, auth, CA, SNI, DB index, and exporter settings. |
| App-side resilience | Redis clients and call sites include timeout/retry profiles, degraded cache behavior, stampede protection, lock ownership checks, and Redis error-budget metrics. |
| Cluster readiness docs | The compatibility audit lists current blockers; the hash-tag convention defines how related keys should be colocated when Cluster mode is introduced. |

## Responsibility Tiers

| Tier | Typical usage | Data-loss tolerance | Required behavior |
| --- | --- | --- | --- |
| R0 Critical coordination | Distributed locks, idempotency, permission-sensitive rate limits | Very low | Fail closed or recover from a durable source of truth. |
| R1 User continuity | Session state, login state, temporary workflow state | Low | Short outage can be tolerated with clear retry behavior. |
| R2 Performance cache | DB/API cache and computed result cache | Medium | Cache miss must not overload the origin. |
| R3 Ephemeral signal | Presence, online status, short-lived dedupe | High | State can be dropped, rebuilt, or delayed. |
| R4 Queue-like usage | Delayed jobs, event buffers, stream/list queues | Depends on the workflow | Business-critical work needs replay, reconciliation, or a durable queue plan. |

## Next Useful Work

| Area | Work |
| --- | --- |
| Key ownership | Keep the dependency map current as new Redis keys are added. |
| Cluster clients | Add explicit Cluster client modes only after the compatibility blockers are addressed. |
| Hash-tag enforcement | Add tests or lint checks for key builders that participate in grouped Redis operations. |
| Inventory coverage | Extend the inventory script to understand Redis Cluster nodes and DB `0` constraints. |
| Operations drills | Keep staging drills focused on failover, restore, memory pressure, latency, auth/TLS config, and degraded app behavior. |

## Documentation Rules

- Do not add production execution records until there is a real production
  environment and an actual change window.
- Do not reference external tracker numbers from Redis docs. Fold the useful
  context into the document itself.
- Do not keep standalone topology-change runbooks for environments that never
  existed.
- Keep rollback guidance local to the active operational procedure it belongs
  to, such as a deployment or incident runbook.
