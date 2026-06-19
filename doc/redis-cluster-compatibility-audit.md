# Redis Cluster Compatibility Audit

Issue: #345

This audit records current Redis Cluster blockers and unknowns only. It does
not change runtime behavior.

The approved key hash-tag convention for follow-up work lives in
`doc/redis-key-hash-tag-convention.md`.

## Scope

Audited code and operational surfaces:

- Go backend Redis client wiring, middleware, auth, dashboard caches, publish
  queues and locks, browser-session coordination, media-asset cache
  invalidation, and read-model/email queues.
- Browser worker Redis state store.
- TypeScript collab-service Redis pub/sub wiring.
- Docker and Kubernetes Redis configuration, including Traefik rate-limit Redis
  settings and the keyspace inventory script.
- Current Redis availability documentation and dependency map.

## Summary

The application is not Redis Cluster ready today. The primary blockers are:

- All first-party Go Redis clients are built with `redis.NewClient` or
  `redis.NewFailoverClient`; the collab service uses `createClient` or
  `createSentinel`. None of the deployed app paths build Cluster-aware clients.
- `REDIS_DB` and `TRAEFIK_RATE_LIMIT_REDIS_DB` are first-class configuration.
  Redis Cluster supports database `0` only, so any non-zero DB config is a
  migration blocker.
- Several Lua scripts touch multiple keys without hash tags that guarantee a
  single slot. Some scripts also derive Redis keys from `ARGV`, which is unsafe
  in Cluster because the accessed key is not declared in `KEYS`.
- Cache invalidation and token cleanup scan for keys and then call variadic
  `DEL` over all matches. Those batches can span slots.
- Browser-session cleanup uses a global sorted set plus per-session keys.
  It works on standalone Redis but needs a Cluster-safe ownership/key-slot
  design.
- Asynq has Redis Cluster support, but MPP currently injects standalone
  `*redis.Client` instances into Asynq. Queue behavior must be wired and tested
  with Cluster clients before production Cluster cutover.

## Blockers And Fix Paths

| ID | Owner | Key pattern or responsibility area | Finding | Risk | Proposed fix path |
| --- | --- | --- | --- | --- | --- |
| RC-01 | backend platform/runtime | `REDIS_ENDPOINT_MODE`, `REDIS_ADDR`, role clients in `backend/internal/redisclient/redisclient.go` | Backend supports direct and Sentinel only, using `redis.NewClient` and `redis.NewFailoverClient`. | Clients will not follow `MOVED`/`ASK` redirects and cannot discover Cluster topology. | Add an explicit `cluster` endpoint mode, parse seed nodes, construct `redis.NewClusterClient`, and preserve role-specific timeout/retry/TLS settings. |
| RC-02 | browser-worker | `browser-worker/internal/session/redis_state.go` | Browser worker also supports direct and Sentinel only. | Worker live-session and heartbeat writes fail or misroute in Cluster. | Add the same Cluster endpoint mode and shared config contract as backend, or factor common Redis config into a reusable module. |
| RC-03 | collab-service | `collab-service/src/collab/redis-pubsub.ts` | Collab service uses node-redis direct or Sentinel clients, not `createCluster`. | Cross-instance collaboration sync will not handle Cluster redirects or topology changes. | Add a Cluster mode using node-redis `createCluster`, define seed-node config, and validate pattern subscription behavior against the target provider. |
| RC-04 | deploy/ops | `REDIS_DB`, `TRAEFIK_RATE_LIMIT_REDIS_DB` | Backend, browser-worker, collab-service, keyspace inventory, Traefik, docs, and contracts expose DB index configuration. | Redis Cluster only supports DB `0`; non-zero configuration fails at connect time or silently points to an impossible topology. | For Cluster mode, hard-reject non-zero DB values, remove non-zero examples, and document DB isolation replacement through key prefixes or separate clusters. |
| RC-05 | backend stream gate | `mpp:stream:conn:*`, `mpp:stream:{kind}:user:*`, `mpp:stream:{kind}:tenant:*`, `mpp:stream:{kind}:ip:*`, `mpp:stream:{kind}:global` | Acquire and release Lua scripts touch five keys with no shared hash tag. | `CROSSSLOT` failures break stream admission control for AI and browser streams. | Redesign the limiter as single-slot per limit group, use a hash-tag convention, or split checks into per-scope operations with an explicit consistency tradeoff. |
| RC-06 | backend app rate limit | `mpp:ratelimit:*` | Rate limiting uses Lua per bucket, which is single-key today, but checks multiple buckets sequentially. | The Lua script itself is Cluster-compatible when each call has one key, but Cluster mode may route several sequential bucket calls across slots and degrade latency/failure semantics. | Keep the one-key script, add integration coverage against Redis Cluster, and decide whether bucket keys should share a tenant/user hash tag for locality. |
| RC-07 | backend auth | `auth:code:*`, `auth:code_attempts:*`, `auth:last_send:*` | Verification cleanup calls `DEL codeKey attemptKey` for distinct key prefixes. | Multi-key `DEL` can fail with `CROSSSLOT`; failed verification cleanup can leave stale codes or attempts. | Hash-tag related code and attempt keys by email digest or replace variadic delete with per-key deletes. |
| RC-08 | backend publish locks | `mpp:publish:lock:{project_id}:{platform}` | Publish lock refresh/release Lua is single-key and Cluster-safe in isolation. Queue and lock clients are currently standalone-only. | Lock commands cannot run until Cluster clients are wired. Queue/lock colocation is also undefined. | Move the coordination client to Cluster mode and decide whether publish lock keys use a `{project_id}` or `{project_id}:{platform}` hash tag for future grouped operations. |
| RC-09 | backend Asynq queues | `asynq:{queue}:*`, `asynq:*`; publish, email, read-model queues | Asynq supports Redis Cluster options, but MPP constructs Asynq clients and servers from injected standalone `*redis.Client` values. | Workers will not operate against Cluster; Asynq internals use Lua and blocking queue semantics that require Cluster-specific configuration and tests. | Create queue clients through Asynq Cluster options or a `redis.ClusterClient`, keep queue keys in one slot per queue, and run publish/email/read-model worker tests against Redis Cluster. |
| RC-10 | backend browser-session active lock | `mpp:browser:active:{user_id}:{platform}` | Active-session release Lua is single-key. | The command is structurally Cluster-safe, but client mode and hash-tag convention are missing. | Use Cluster clients and add `{user_id}` or `{session_id}` tags according to the final browser-session key model. |
| RC-11 | backend browser-session quota | `mpp:browser:quota:user:{user_id}`, `mpp:browser:quota:tenant:{tenant_id}` | Quota acquire Lua touches user and tenant sorted sets together. Release uses a pipeline with two `ZREM` commands. | Lua fails with `CROSSSLOT`; pipeline may route incorrectly or lose the expected all-or-nothing feel. | Either split user and tenant quotas into independent operations with compensating cleanup, or define a single hash tag for both quota keys. Because user and tenant IDs differ, this needs a product decision on the authoritative quota dimension. |
| RC-12 | backend/browser-worker live browser state | `mpp:browser:session:{session_id}`, `mpp:browser:worker-heartbeat:{worker_session_ref}` | Backend and browser-worker share live session and heartbeat keys, but no hash-tag convention exists. | Single-key reads/writes survive after Cluster clients exist, but recovery flows that read both keys may cross slots and become harder to reason about. | Define a browser-session hash tag, preferably `{session_id}`, and include the session ID in heartbeat keys or add an index that maps worker refs to session IDs safely. |
| RC-13 | backend browser-session cleanup index | `mpp:browser:cleanup` plus `mpp:browser:session:*` | Cleanup uses one global sorted set of session IDs and then deletes per-session keys. | A global index becomes a single-slot hot key if tagged, or cross-slot coordination if not tagged. | Decide whether cleanup remains a global singleton, moves to DB-driven cleanup, or shards cleanup indexes by hash tag/time bucket. |
| RC-14 | backend browser stream tokens | `mpp:browser:stream-current:{session_id}`, `mpp:browser:stream-token:{session_id}:{token_hash}` | Token rotation and consume Lua touch current and token keys; rotation also deletes an old token key constructed from `ARGV`. `deleteRedisStreamToken` scans by prefix and then variadic-deletes matches. | Lua can fail with `CROSSSLOT`, and `ARGV` key access is unsafe in Cluster. Cleanup `DEL` can also cross slots. | Hash-tag all stream token keys by `{session_id}`, declare all accessed keys in `KEYS`, or replace dynamic delete with a separate command. Delete scanned keys one at a time or by same-slot batches only. |
| RC-15 | backend dashboard project-list cache | `mpp:dashboard:projects:list:v2:*`, `mpp:dashboard:projects:list-generation:v2` | Cache reads/writes are single-key, but invalidation scans and variadic-deletes cache keys while separately incrementing a generation key. | Invalidation `DEL` can fail across slots; generation and data keys may diverge during partial failures. | Prefer generation-only invalidation, or hash-tag each cache family and delete same-slot batches. |
| RC-16 | backend content-setup cache | `mpp:dashboard:content-setup:v1:*` | Reads/writes and generation keys are single-key; invalidation scans by user/workspace pattern and variadic-deletes matches. | Cross-slot `DEL` during invalidation; user/workspace generation keys are not colocated with all affected cache entries. | Use generation-only invalidation, or introduce hash tags per workspace/user and split deletes by slot. |
| RC-17 | backend media-asset resolve cache | `mpp:dashboard:media-assets:resolve:v1:{asset_id}:actor:{user_id}` | Invalidation scans all actors for one asset and variadic-deletes matches. | Cross-slot `DEL` if actor IDs cause separate slots. | Hash-tag by `{asset_id}` or delete each scanned key independently. |
| RC-18 | backend dashboard stats and account caches | `mpp:dashboard:stats:*`, `mpp:dashboard:accounts:v1:*` | Current operations are single-key `GET`, `SET`, `INCR`, or single-key `DEL`. | No command-level blocker once Cluster clients exist, but no key-tag convention exists for future grouped operations. | Mark as compatible after Cluster-client integration tests; document prefix and hash-tag rules for new keys. |
| RC-19 | backend X OAuth2 state | `mpp:x_oauth2_state:{state}` | Uses `SET NX` and `GETDEL` on one key. | No command-level blocker once Cluster clients exist. | Mark as compatible after Cluster-client integration tests. |
| RC-20 | collab-service pub/sub | `mpp:collab:doc:*` channels | Uses pattern subscription and publish. | Redis Cluster pub/sub semantics depend on provider and client mode. Pattern subscription may not deliver across all shards unless the Cluster client manages node subscriptions as expected. | Validate node-redis Cluster pub/sub against the chosen provider. Consider sharded pub/sub or one channel hash tag per document if provider support requires it. |
| RC-21 | Traefik gateway rate limit | Docker labels for `ratelimit.redis.*` | Traefik rate-limit Redis config uses endpoint/password/db labels and no Cluster-specific shape in this repo. | Gateway throttling may not support or may need different config for Redis Cluster. | Verify Traefik v3 Redis rate-limit middleware Cluster support for the deployed build. If unsupported, keep gateway rate-limit Redis on standalone/managed HA or move gateway limits elsewhere. |
| RC-22 | scripts and runbooks | `script/redis/keyspace_inventory.rb`, migration runbooks | Inventory scans one logical DB via `redis-cli -u .../{db}` and does not enumerate Cluster nodes. | Cluster inventory can miss keys or fail on unsupported DB selection. | Add Cluster inventory mode that uses `CLUSTER SLOTS` or provider APIs, scans each primary, and forces DB `0`. |

## Required Category Coverage

| Category | Coverage result |
| --- | --- |
| Multi-key commands | Blockers found in stream gate Lua, browser-session quota Lua, browser stream token Lua/cleanup, auth `DEL`, cache scan-and-delete invalidation, and Asynq queue internals. |
| Lua scripts | Blockers found in stream gate, browser quota, browser token rotation/consume, and dynamic key access from `ARGV`. Single-key scripts in publish locks, app rate limits, and active-session release are structurally compatible after Cluster clients are added. |
| Transactions | No first-party `MULTI`, `WATCH`, `TxPipeline`, or `TxPipelined` usage was found. Asynq internals still need Cluster certification because they use Lua and queue state transitions internally. |
| DB index usage | `REDIS_DB` and `TRAEFIK_RATE_LIMIT_REDIS_DB` are configured across app services, contracts, scripts, and docs. Cluster mode must force DB `0`. |
| Blocking commands | No first-party `BLPOP`, `BRPOP`, `BRPOPLPUSH`, `BZPOP*`, or `XREAD` calls were found. Asynq worker dequeue behavior is library-managed and must be tested in Cluster mode. |
| Pipelines | First-party pipeline use found in browser-session quota release with two keys. Go-redis Cluster clients can route pipelines differently from standalone clients, so this should be split or same-slot constrained. |
| Standalone assumptions | Direct/Sentinel-only client wiring, single logical DB config, standalone `redis-cli` inventory, and single-endpoint Traefik rate-limit wiring all assume non-Cluster Redis. |

## Critical Unknowns

| Unknown | Why it matters | Decision or investigation path |
| --- | --- | --- |
| Final hash-tag dimension | Browser sessions and stream gates group keys by user, tenant, session, IP, global scope, and connection ID. These dimensions do not naturally share one slot. | PR 5.2 should choose key tags per responsibility, with explicit tradeoffs for global counters and tenant/user limits. |
| Asynq Cluster operating mode | Asynq provides Cluster support but MPP uses injected standalone clients and has not validated worker behavior against Cluster. | PR 5.4 or 5.5 should run publish, email, and read-model queue tests against Redis Cluster using Asynq Cluster options. |
| Collab pub/sub provider behavior | Cluster pub/sub semantics vary by Redis version/provider and node-redis client mode. | Validate pattern subscription across at least two shards in non-prod. Decide between classic Cluster pub/sub, sharded pub/sub, or a non-Redis fan-out path. |
| Gateway rate-limit support | Traefik Redis rate-limit Cluster behavior is not proven by repo code. | Confirm Traefik middleware support against the exact deployed Traefik version before moving gateway limits to Cluster. |
| Global cleanup and global limits | Redis Cluster makes global sorted sets/counters natural hot spots if kept in one slot, but cross-slot global operations fail. | Decide whether global stream limits and browser cleanup stay centralized, are sharded with approximate limits, or move to a durable DB/worker model. |
| Data migration tooling | Current inventory and migration scripts are standalone/Sentinel oriented. | Add Cluster-aware inventory and migration rehearsal before any non-prod Cluster deployment. |

## Recommended Follow-Up Order

1. Define the Redis Cluster key hash-tag convention for each responsibility area.
2. Add Cluster endpoint mode and Cluster-aware clients for backend,
   browser-worker, and collab-service.
3. Replace or constrain multi-key Lua and variadic `DEL` paths.
4. Wire Asynq queues through Cluster-compatible options and test worker flows
   against a real Redis Cluster.
5. Validate collab pub/sub, Traefik rate limiting, and keyspace inventory
   against the selected provider.

## Audit Evidence

Representative files reviewed:

- `backend/internal/redisclient/redisclient.go`
- `browser-worker/internal/session/redis_state.go`
- `collab-service/src/collab/redis-pubsub.ts`
- `backend/internal/pkg/streamgate/streamgate.go`
- `backend/internal/middleware/rate_limit.go`
- `backend/internal/handlers/auth.go`
- `backend/internal/services/browser_session/redis.go`
- `backend/internal/services/browser_session/tokens.go`
- `backend/internal/services/publish/queue.go`
- `backend/internal/services/email/queue.go`
- `backend/internal/services/readmodel/queue.go`
- `backend/internal/services/project/list_cache.go`
- `backend/internal/services/project/setup_options_cache.go`
- `backend/internal/services/mediaasset/resolve_cache.go`
- `deploy/docker/docker-compose.yml`
- `contracts/env.schema.yaml`
- `script/redis/keyspace_inventory.rb`

Searches explicitly checked for multi-key commands, Lua scripts, transactions,
DB-index configuration, blocking commands, pipelines, pub/sub, Asynq usage, and
standalone Redis client construction.
