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

- Redis timeout/retry baseline now follows client role rather than one shared profile:
  - `R0` coordination (`streamgate`, publish locks, browser-session ownership, browser stream token checks): 500ms dial/read/write, 750ms pool wait, no automatic command retry, single short dial retry.
  - `R1` continuity (`auth` verification codes, X OAuth2 callback state, browser live-session continuity, browser-worker live-session state): 750ms dial, 1s read/write, 1250ms pool wait, 1 bounded retry with 25-150ms backoff.
  - `R2` caches: 750ms dial/read/write, 1s pool wait, 1 bounded retry with 25-150ms backoff.
  - `R4` queues and async workers: 1s dial, 2s read/write, 2s pool wait, 2 bounded retries with 50-250ms backoff.
  - `R3` collab Redis pub/sub sync uses node-redis bounded reconnect with 1s connect/socket timeout, disabled offline queue, max queue length 256, and reconnect delay capped at 2s.
- `R0` items need explicit recovery semantics before any topology change. Publish locks and stream gates are the main ones.
- `R1` items should preserve user continuity or fail with a clear retry path. Browser sessions and auth codes are the most visible.
- `R4` items need durable replay or a non-Redis queue before Redis Cluster work.
- Cache paths are `R2` and should stay best-effort.

## R0 Coordination Safety Semantics

| Path | Acquire | Refresh | Release | Failover or recovery behavior |
| --- | --- | --- | --- | --- |
| Publish lock `mpp:publish:lock:{project_id}:{platform}` | The backend acquires the lock with Redis `SET NX` and an explicit 30-minute TTL. The owner token is the publish job UUID, which is also present in the durable publish event/outbox trail and acts as the ownership fence for duplicate enqueue/replay decisions. | The worker refreshes only when the stored value still equals the job UUID. If refresh observes a different owner, the publish context is canceled so the worker stops assuming it still owns the publication. Refresh errors are logged and retried, but the lock TTL remains the final fail-closed recovery boundary. | Release uses compare-and-delete Lua and removes the key only when the stored value still equals the job UUID. Stale workers cannot release a newer owner's lock. | If Redis loses the lock during failover, a worker may reacquire only when the publication row is still in a retriable queued/publishing/failed state. Duplicate requests replay durable publish events by idempotency key instead of creating another owner silently. |
| Stream gate leases `mpp:stream:*` | The limiter creates a random connection owner ID, writes a TTL-bound connection key, and records that owner in per-user, tenant, IP, and global sorted sets. Acquire prunes expired owners before checking limits and fails closed when Redis returns an unexpected result. | Stream leases are not refreshed. The configured TTL is the upper bound for stale admission state if a client or backend instance disappears. | Release is owner-checked against the connection-key payload before deleting the connection key or sorted-set members. A stale release is a no-op if the owner payload has changed or the connection key is gone. | After Redis failover or key loss, admission counters rebuild from new acquisitions. Active streams are not granted stronger continuity by Redis; the gate is a concurrency guard and fails closed on Redis errors when enabled. |
| Browser-session active lock `mpp:browser:active:{user_id}:{platform}` | Session start uses Redis `SET NX` with an explicit session TTL plus grace. The owner token is the browser session ID and the durable PostgreSQL row stores the same ID. | Active-session locks are not refreshed directly; live session state, heartbeat, and cleanup indexes carry their own TTLs. | Release uses compare-and-delete and only removes the active lock when the stored session ID still matches. Stream tokens are single-use and consumed with Lua that also clears the current-token pointer only when it still references the consumed token. | Start-session recovery reads the active lock owner, live session state, worker heartbeat, and worker API. Missing, expired, terminal, or unreachable owners are expired in PostgreSQL and cleaned from Redis before a new session is admitted. |

## Linked From

- [Redis availability plan](./plan/redis-availability-plan.md)
