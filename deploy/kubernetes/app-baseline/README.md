# Kubernetes App Baseline

This package is a renderable workload base, not a standalone production
deployment. Environment overlays must provide the cluster-specific pieces before
applying it to a real cluster.

Required overlay inputs:

- Registry-qualified immutable images for every
  `registry.example.invalid/...:replace-me` image. The repository publishes
  GHCR images with `sha-<full-git-sha>` tags; use those immutable tags in
  production overlays.
- A reachable PostgreSQL endpoint through `DB_HOST`/`DB_PORT` or an in-cluster Service.
- PostgreSQL TLS policy through `DB_SSLMODE`; the baseline defaults to
  `verify-full` for managed production databases. Set `DB_SSLROOTCERT` when a
  provider or private CA requires a custom root certificate path.
- A reachable Redis endpoint through `REDIS_ADDR` or an in-cluster Service.
- Public collaboration routing through `COLLAB_WEBSOCKET_URL_BASE`.
- Public HTTP routing through the `mpp-public-gateway` Ingress. The baseline
  routes `/collab` to `collab-service` for WebSocket traffic and all remaining
  paths to the Next.js frontend, which proxies backend API calls.
- A TLS Secret referenced by the Ingress. Patch `spec.ingressClassName`, host,
  and `spec.tls[*].secretName` for the target cluster.
- LLM provider configuration through `LLM_PROVIDER_URL`, `LLM_MODEL`, and
  `LLM_PROVIDER_KEY`.
- Browser runtime control resources from
  `deploy/kubernetes/browser-runtime-control`, including the runtime namespace,
  browser-worker ServiceAccount, RoleBinding, admission policy, and
  NetworkPolicy.
- A `mpp-app-secrets` Secret in `mpp-system` with at least `JWT_SECRET`,
  `DB_PASSWORD`, `COLLAB_TOKEN_SECRET`, `COOKIE_ENCRYPTION_KEY`, and
  `LLM_PROVIDER_KEY`.

The CI validation overlay under `deploy/kubernetes/validation/app-baseline`
uses fake values to verify manifest shape without committing real secrets or
pretending that production data services and image publishing already exist.

The `mpp-backend` image intentionally serves both the backend API and
publish-worker Deployments; keep those two image references aligned when
patching an environment overlay.

The optional data service packages under `deploy/kubernetes/data-services`
provide either stable DNS aliases for managed PostgreSQL and Redis or minimal
self-hosted StatefulSets for non-production clusters.
