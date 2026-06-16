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
- Patch `mpp-data-backups` storage, backup CronJob schedules, and
  `BACKUP_RETENTION_DAYS` before keeping useful data in the StatefulSets.

The included NetworkPolicies allow PgBouncer ingress from backend,
publish-worker, and collab-service Pods; PostgreSQL ingress from those app Pods
plus PgBouncer and the PostgreSQL backup CronJob; and Redis ingress from
backend, publish-worker, browser-worker, collab-service, and the Redis backup
CronJob Pods.

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
