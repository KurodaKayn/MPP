# Retired Production HA Redis Package

This package used to render the production self-hosted HA Redis topology for
the Phase 2 cutover. It is retired by issue #339 after production moved to
managed Redis and completed the agreed soak.

Do not apply this package to production to recreate Redis. It now renders only
the `redis-ha-production-retired` ConfigMap, which records that
`deploy/kubernetes/overlays/production-managed` is the active production Redis
path.

Use `doc/self-hosted-redis-decommission-record.md` for no-traffic evidence,
retained backup details, deletion steps, and recreate-from-history rollback
notes during the retention window. The non-production HA validation package at
`deploy/kubernetes/data-services/redis-ha-nonprod` remains available for
staging drills and Redis HA behavior tests.

Validation:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/data-services/redis-ha-production > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/data-services/redis-ha-production \
  "$rendered"
```
