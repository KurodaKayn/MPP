# Managed Data Services

Use this package when PostgreSQL and Redis are provided by a managed service
outside the cluster. It gives the application stable Kubernetes DNS names while
keeping backup, retention, failover, and maintenance responsibilities with the
managed provider.

Required overlay inputs:

- Patch the `postgres` ExternalName to the managed PostgreSQL host.
- Patch the `redis` ExternalName to the managed Redis host.
- Set `DB_HOST=postgres` and `REDIS_ADDR=redis:6379` in `mpp-app-config`.
- Keep `DB_SSLMODE=verify-full` for production managed PostgreSQL. Set
  `DB_SSLROOTCERT` when the provider requires a custom CA bundle mounted into
  the app pods.
- Provide `DB_PASSWORD` and, when Redis auth is enabled, `REDIS_PASSWORD` in
  `mpp-app-secrets`.
- Set `REDIS_TLS=true` when the managed Redis endpoint requires TLS.

Do not store provider credentials in this package. Materialize them through a
Kubernetes Secret or an external secret manager.
