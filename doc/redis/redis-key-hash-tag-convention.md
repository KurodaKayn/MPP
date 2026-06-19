# Redis Key Hash-Tag Convention

This convention defines how MPP names Redis keys that may need Redis Cluster
single-slot behavior. It complements the Redis Cluster compatibility audit in
`doc/redis/redis-cluster-compatibility-audit.md`.

## Rules

- Use a Redis hash tag when keys can be passed together to a multi-key command,
  Lua script, transaction, pipeline, scan-and-delete batch, or any future grouped
  operation that requires same-slot behavior.
- Use the shape `{scope:value}`. Approved scopes are `tenant`, `user`,
  `session`, `project`, `asset`, `email`, `queue`, and narrow cache-family
  names such as `dashboard:projects-list`.
- Choose the smallest stable identifier shared by every key in the group. Do
  not choose a tag that changes quota, counter, or cache invalidation semantics.
- Build backend keys with `backend/internal/pkg/rediskey.Tag` and normalize
  untrusted key parts with `rediskey.Part`.
- Single-key-only Redis operations may omit a tag, but new keys should still use
  a tag when the responsibility area has an approved future grouping dimension.
- Do not put raw user input inside `{...}`. Hash or canonicalize sensitive
  values first, as auth verification keys do with the canonical email digest.

## Examples

```text
auth:code:{email:<sha256_email>}:register
auth:code_attempts:{email:<sha256_email>}:register
mpp:browser:session:{session:<session_id>}
mpp:browser:stream-token:{session:<session_id>}:<token_hash>
mpp:publish:lock:{project:<project_id>}:wechat
mpp:dashboard:media-assets:resolve:v1:{asset:<asset_id>}:actor:<user_id>
```

## Audit Strategy Table

| Audit ID | Area | Approved hash-tag strategy |
| --- | --- | --- |
| RC-05 | Stream gate Lua | No tag is approved for the current five-key Lua shape. User, tenant, IP, and global counters aggregate across different dimensions, so a forced shared tag would change limiter semantics. Split per-scope operations or move global coordination before Redis Cluster is enabled. |
| RC-06 | App rate limit | Current Lua calls are single-key. If a future operation groups buckets, tag by the authoritative bucket owner, such as `{user:<user_id>}` or `{ip:<ip_hash>}`. |
| RC-07 | Auth verification | Use `{email:<sha256_canonical_email>}` across code, attempts, and last-send keys. Implemented in backend auth key builders. |
| RC-08 | Publish locks | Use `{project:<project_id>}` for publish locks so future project-scoped queue/lock coordination has a stable slot. Implemented in publish lock keys. |
| RC-09 | Asynq queues | Keep Asynq's own queue hash tag, such as `asynq:{publish}:...`, and wire queues through Asynq Cluster options before Redis Cluster is enabled. |
| RC-10 | Browser active lock | Use `{user:<user_id>}` for the active user/platform lock. Implemented in browser-session coordination keys. |
| RC-11 | Browser quota | No shared tag is approved yet. User and tenant quota keys aggregate on different dimensions; tagging both by either side would weaken one quota. Keep this as a design decision for the quota rewrite. |
| RC-12 | Browser live state and heartbeat | Use `{session:<session_id>}` for live session state. Worker heartbeats remain single-key by worker ref until the worker-ref-to-session index is redesigned. |
| RC-13 | Browser cleanup index | No shared tag is approved for the global cleanup sorted set. It remains a global index until cleanup is moved to DB-backed or sharded ownership. |
| RC-14 | Browser stream tokens | Use `{session:<session_id>}` across current-token and token keys. Implemented for token keys; declaring every Lua key in `KEYS` remains part of the Cluster command rewrite. |
| RC-15 | Dashboard project-list cache | Use `{dashboard:projects-list}` across list data keys and the generation key. Implemented in project-list cache builders. |
| RC-16 | Content setup cache | No single tag is approved because user-scoped and workspace-scoped invalidations overlap. Prefer generation-only invalidation or split invalidation by a single authoritative scope. |
| RC-17 | Media asset resolve cache | Use `{asset:<asset_id>}` for all actor-specific resolve cache entries for one asset. Implemented in media-asset cache keys. |
| RC-18 | Stats and account caches | Current operations are single-key. Future grouped user caches use `{user:<user_id>}`; grouped workspace caches use `{workspace:<workspace_id>}`; truly global cache families use a narrow family tag. |
| RC-19 | X OAuth2 state | Current `GETDEL` flow is single-key. No tag is required unless a future cleanup groups state keys by user or workspace. |
| RC-20 | Collab pub/sub | Current channels are pub/sub, not multi-key data commands. If sharded pub/sub is adopted, use `{doc:<document_id>}` in document channels. |
| RC-21 | Traefik gateway rate limit | No MPP key shape is approved until Traefik Cluster behavior is verified for the deployed build. Keep gateway Redis rate-limit keys outside this convention for now. |
| RC-22 | Keyspace inventory | Inventory patterns recognize the implemented tagged keys. Cluster-aware scanning still needs per-primary DB 0 inventory support. |

## Automated Checks

- `backend/internal/pkg/rediskey` tests verify tag construction and same-tag
  detection.
- Backend package tests assert implemented multi-key groups keep their approved
  tags.
- Browser-worker tests assert shared browser live-session keys use the same
  `{session:<session_id>}` tag shape as backend.
- `script/redis/test_keyspace_inventory.rb` verifies the inventory recognizes
  tagged key patterns.
