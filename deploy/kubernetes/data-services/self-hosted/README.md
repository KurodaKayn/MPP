# Self-Hosted Data Services

Use this package for test clusters, small self-hosted installations, or demos
where managed PostgreSQL and Redis are not available. Production deployments
should prefer managed services with provider-backed backup, maintenance, and
failover.

Required overlay inputs:

- Provide `mpp-app-secrets` with `DB_PASSWORD`. `REDIS_PASSWORD` is optional,
  but recommended.
- Set `DB_HOST=pgbouncer` and `REDIS_ADDR=redis:6379` in `mpp-app-config`.
  PgBouncer uses the in-cluster `postgres` Service as its writer upstream.
- Patch `DB_SSLMODE=disable` unless you add TLS certificates to the self-hosted
  PostgreSQL StatefulSet.
- Patch storage class, storage sizes, resource limits, and image tags for the
  target cluster.
- Review the `redis-persistence-config` ConfigMap before applying an overlay.
  The base policy stores Redis files on the `redis-data` PVC, enables AOF with
  `appendfsync everysec`, and keeps the default RDB snapshot cadence for
  restart recovery and backup snapshots.
- Patch `mpp-data-backups` storage, backup CronJob schedules, and
  `BACKUP_RETENTION_DAYS` before keeping useful data in the StatefulSets.

The included NetworkPolicies allow PgBouncer ingress from backend,
publish-worker, and collab-service Pods; PostgreSQL ingress from those app Pods
plus PgBouncer and the PostgreSQL backup CronJob; and Redis ingress from
backend, publish-worker, browser-worker, collab-service, and the Redis backup
CronJob Pods.

Redis runs as a single-pod StatefulSet with persistent storage:

- `redis-data-redis-0` mounts at `/data` and stores `dump.rdb` plus the AOF
  directory.
- `redis-persistence-config` is mounted read-only at `/usr/local/etc/redis` and
  is the versioned source of the Redis persistence mode.
- Readiness and liveness probes run `redis-cli ping` with optional
  `REDIS_PASSWORD` support. Readiness fails when Redis stops serving commands,
  instead of only checking whether the TCP port is open.
- The default Redis container requests `100m` CPU and `256Mi` memory, with
  limits of `500m` CPU and `512Mi` memory. Patch these values in the target
  overlay when cluster capacity or workload size needs different headroom.
- Redis gets a 60-second termination grace period. The `preStop` hook runs
  `SHUTDOWN SAVE` with optional password auth so normal Pod deletion or restart
  asks Redis to flush before Kubernetes forcefully terminates the container.
- AOF is enabled with `appendfsync everysec`, so a normal Pod restart should
  keep Redis-resident keys that have not expired. A node or storage failure may
  still lose writes accepted inside the last fsync window.
- RDB snapshots remain enabled with `save 900 1`, `save 300 10`, and
  `save 60 10000`. They complement AOF and provide the snapshot source used by
  the Redis backup CronJob.
- Short-TTL keys can expire during or after restart. Queue-like, lock, session,
  and idempotency keys must still follow their documented Redis responsibility
  tier and recovery expectation.

Environment overlays may patch `redis-persistence-config` only when the
environment intentionally chooses a different RDB/AOF policy and documents the
resulting data-loss expectation in that overlay.

This package includes a small backup starter:

- `postgres-backup` runs `pg_dump --format=custom` every day and stores dumps
  under `/backups/postgres` on the `mpp-data-backups` PVC.
- `redis-backup` runs `redis-cli --rdb` every day and stores RDB snapshots under
  `/backups/redis` on the same PVC.
- Both jobs use `concurrencyPolicy: Forbid`, short active deadlines, restricted
  Pod security settings, no mounted service account token, and local
  file-retention cleanup.

This package also includes the shared `deploy/kubernetes/data-services/redis-exporter`
base. The exporter reads Redis with the optional `REDIS_PASSWORD` Secret and is
allowed through the Redis NetworkPolicy so the observability stack can export
availability, latency, memory, eviction, and blocked-client metrics.

The backup PVC is still in-cluster storage. For production-like self-hosted
use, copy backup artifacts to external object storage or pair the PVC with
storage-provider snapshots before depending on it for disaster recovery.

This package preloads `pg_stat_statements` and mounts an init script that creates
the extension for newly initialized databases. Existing PostgreSQL volumes need a
one-time `CREATE EXTENSION IF NOT EXISTS pg_stat_statements;` run by an
administrator and a PostgreSQL restart for the preload setting to take effect.

The backend remains responsible for schema migration and initialization. Do not
run migrations as Kubernetes manifest side effects.
