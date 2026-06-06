# Self-Hosted Data Services

Use this package for test clusters, small self-hosted installations, or demos
where managed PostgreSQL and Redis are not available. Production deployments
should prefer managed services with provider-backed backup, maintenance, and
failover.

Required overlay inputs:

- Provide `mpp-app-secrets` with `DB_PASSWORD`. `REDIS_PASSWORD` is optional,
  but recommended.
- Set `DB_HOST=postgres` and `REDIS_ADDR=redis:6379` in `mpp-app-config`.
- Patch `DB_SSLMODE=disable` unless you add TLS certificates to the self-hosted
  PostgreSQL StatefulSet.
- Patch storage class, storage sizes, resource limits, and image tags for the
  target cluster.
- Configure backup, restore, and retention outside these manifests.

The backend remains responsible for schema migration and initialization. Do not
run migrations as Kubernetes manifest side effects.
