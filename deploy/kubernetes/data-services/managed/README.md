# Managed Data Services

Use this package when PostgreSQL and Redis are provided by a managed service
outside the cluster. It gives the application stable Kubernetes DNS names while
keeping backup, retention, failover, and maintenance responsibilities with the
managed provider.

Required overlay inputs:

- Patch the `postgres` ExternalName to the managed PostgreSQL host if you want a
  stable in-cluster alias for tooling or non-verifying TLS modes.
- Patch the `postgres-reader` ExternalName to the managed PostgreSQL read
  replica host and set `DB_READER_HOST` to the same provider hostname when using
  `DB_READER_SSLMODE=verify-full` or inherited `DB_SSLMODE=verify-full`.
- Patch the `redis` ExternalName to the managed Redis host.
- Set `DB_HOST` to the managed PostgreSQL provider hostname when
  `DB_SSLMODE=verify-full`, because hostname verification must match the
  provider certificate.
- Set `REDIS_ADDR` to the managed Redis provider hostname and port when
  `REDIS_TLS=true`, because Redis TLS hostname verification must match the
  provider certificate. Keep the `redis` ExternalName Service for stable
  in-cluster discovery when TLS verification is not hostname-sensitive.
- Keep `DB_SSLMODE=verify-full` for production managed PostgreSQL. Set
  `DB_SSLROOTCERT` when the provider requires a custom CA bundle mounted into
  the app pods.
- Provide `DB_PASSWORD` and, when Redis auth is enabled, `REDIS_PASSWORD` in
  `mpp-app-secrets`.
- Set `REDIS_TLS=true` when the managed Redis endpoint requires TLS.
- Set `REDIS_TLS_CA_CERT` or `REDIS_TLS_CA_FILE` when the provider requires
  custom Redis CA trust material. Use `REDIS_TLS_SERVER_NAME` only when the
  provider documents an SNI/certificate name that differs from `REDIS_ADDR`.
- Patch the redis-exporter `REDIS_ADDR` to the managed Redis hostname, using
  `rediss://...` when the provider requires TLS. The exporter reads the same
  optional `REDIS_PASSWORD` Secret and feeds the Redis observability baseline.
- Choose and record the managed Redis persistence policy in the environment
  overlay or provider configuration. For production-like managed Redis, require
  provider-backed persistence, scheduled snapshots, and a documented restore
  point objective instead of relying on any in-cluster PVC.
- Complete `doc/managed-redis-nonprod-validation.md` against a non-production
  managed Redis instance before using this package for production Redis
  migration planning.

Do not store provider credentials in this package. Materialize them through a
Kubernetes Secret or an external secret manager.

For a renderable staging starter that wires this package to the app baseline and
browser runtime controls, see `deploy/kubernetes/overlays/staging-managed`.
