# MPP Redis Dependency Map

This map documents current Redis blast radius only. It records where Redis is used, what breaks or degrades when Redis is unavailable, and which follow-up direction owns each risk. It does not implement fallback logic.

## Scope And Reading Guide

This is a service-level map, not a key inventory. The key inventory lives in `script/redis/keyspace_inventory.rb`; this document groups those patterns by request path and operational effect.

Risk tags:

- `R0`: critical coordination
- `R1`: user continuity
- `R2`: performance cache
- `R3`: ephemeral signal
- `R4`: queue-like usage

## Map

| Service | Redis roles | What happens when Redis is unavailable | Current behavior | Follow-up direction |
| --- | --- | --- | --- | --- |
| `backend` API | `R0` stream gates, `R0` publish locks, `R1` auth verification state, `R1` browser session continuity, `R2` dashboard caches, `R2` media-asset resolve cache, `R2` platform-account cache, `R3` rate limits, `R4` Asynq queues | In running services, cache reads fall back to DB or object storage, but coordination paths fail closed or return `503`/`429`. Publish can keep DB rows for scheduled/outbox work, yet live Redis dispatch stops. If Redis is missing before boot in a required deployment, the runtime cannot wire the Redis-backed pieces | Most cache reads fall back to DB/object storage. Several coordination paths fail closed or return `503`/`429`. Publish can persist scheduled/outbox rows when Redis queueing is absent at startup, but live worker dispatch is lost until Redis returns | Harden `R0` with explicit recovery and fencing; keep `R1` user-retry UX clear; move important `R4` work off Redis or add durable replay guarantees |
| `publish-worker` | `R4` Asynq publish queue, `R0` publish locks, `R4` outbox dispatch target | If Redis is unavailable after boot, queue consumption and lock refresh stop. If Redis is missing before boot in the worker deployment, the process fails to wire its required Redis client | The worker is Redis-required. DB-backed outbox and schedule rows preserve intent, but job dispatch is blocked while Redis is unavailable | Keep durable outbox replay; consider a queue with explicit durability guarantees for critical publish work |
| `browser-worker` | `R1` browser live-session state, `R3` worker heartbeats, `R3` cleanup index | If Redis is unavailable after boot, ready checks fail, live-session writes stop, recovery from worker loss weakens, and cleanup degrades. If Redis is missing before boot in the configured deployment, startup fails; if it is intentionally omitted, the worker can run in a reduced in-memory mode | `/ready` returns not-ready when Redis ping fails. In reduced mode, session creation still proceeds, but live-session metadata, heartbeat, and cleanup semantics are weaker | Keep Redis as liveness/indexing only, or add stronger DB-backed session recovery before relying on it for continuity |
| `collab-service` | `R3` Redis pub/sub sync | If Redis sync is enabled and Redis is unavailable, startup fails while connecting/subscribing. If sync is disabled, collaboration stays local to one instance and cross-instance updates stop | Redis sync is config-gated. Enabled deployments rely on Redis for instance-to-instance update propagation | Decide whether cross-instance collab needs durable fan-out or whether instance-local sync is acceptable |
| `traefik` gateway | `R3` gateway rate-limit counters | Gateway-level throttling loses its shared Redis state; product services behind the gateway may still be reachable, but ingress protection is weakened or depends on Traefik's own failure behavior | Docker deployment configures Traefik's rate-limit middleware with Redis endpoints and password | Keep gateway rate-limit failure behavior documented with ingress operations, and keep app-side rate limits independent |
| `redis-exporter`, dashboards, and alerts | `R2` observability of Redis health | Redis metrics disappear; alerting and SLO visibility degrade rather than product flows | Observability fails open for product traffic but loses visibility into Redis health | Keep exporter and dashboards available on the same Redis endpoint, and make alerting independent of product request paths |
| `redis-backup` job | `R1`/`R4` recovery artifact creation | Redis snapshots cannot be produced while Redis is unavailable; current product traffic impact depends on app paths above | Backup job uses `redis-cli --rdb` against the Redis service and stores RDB snapshots | Tie backup failure alerts to restore readiness before moving critical queues or user-continuity state deeper into Redis |

## Request-Level Effects

| Request path or workflow | Redis dependency | When Redis fails | Owner |
| --- | --- | --- | --- |
| Send verification code / reset password | auth verification keys (`R1`) | Request returns `503`; the code service is unavailable | backend auth handler |
| Verify code | auth verification keys (`R1`) | Invalid/expired behavior becomes unavailable; retries cannot be tracked in Redis | backend auth handler |
| Start browser session | live session lock, quota, token, heartbeat, cleanup (`R1`, `R3`) | Session start may fail or lose Redis-backed recovery and quota tracking; stale sessions can linger until DB cleanup catches up | backend browser session service |
| Get browser session status | live session state + heartbeat (`R1`, `R3`) | Status degrades to DB-only, and live worker loss is detected less precisely | backend browser session service |
| Browser stream proxy | stream gate lease (`R0`) | Admission control fails closed or returns `503`; the stream may still work only when the limiter is disabled or memory-backed | backend stream gate |
| Publish or schedule a project | publish lock + queue (`R0`, `R4`) | Locking or queue dispatch fails when Redis is down after boot; if Redis is absent at boot, scheduled/outbox rows still persist in PostgreSQL and dispatch waits for Redis to return | backend publish service |
| Flush publish outbox / scheduled jobs | Redis queue (`R4`) | Redis dispatch stops and pending jobs accumulate in PostgreSQL | backend publish service |
| Dashboard project list / stats / accounts / media resolve | caches (`R2`) | Requests recompute from PostgreSQL or object storage; latency increases, but user flows continue | backend dashboard services |
| Resolve OAuth2 callback state | state store (`R1`) | Pending connection callbacks cannot be consumed; the user must restart the connection | backend platform account service |
| Send queued verification/password-reset email | Asynq email queue (`R4`) | When async email is Redis-backed, enqueue or worker drain fails; code state may exist but email delivery is delayed or failed | backend email service |
| Rebuild dashboard read models | Asynq read-model queue (`R4`) | Admin rebuild requests return unavailable or worker drain stops; normal DB-backed reads continue | backend dashboard service |
| Gateway request admission | Traefik rate-limit counters (`R3`) | Gateway-wide throttling loses shared Redis state; backend app rate limits still apply only when their own Redis path is healthy | gateway operations |
| Collab document sync between instances | pub/sub sync (`R3`) | Cross-instance updates stop propagating; local editing still continues | collab-service |

## Owner And Follow-Up Notes

- `R0` items need explicit recovery semantics before any topology change. Publish locks and stream gates are the main ones.
- `R1` items should preserve user continuity or fail with a clear retry path. Browser sessions and auth codes are the most visible.
- `R4` items need durable replay or a non-Redis queue before Redis Cluster work.
- Cache paths are `R2` and should stay best-effort.

## Linked From

- [Redis availability plan](./plan/redis-availability-plan.md)
