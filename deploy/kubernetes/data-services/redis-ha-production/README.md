# Production HA Redis Package

This package is not an active deployment path for the project.

The checked-in package renders only the `redis-ha-production-retired` ConfigMap.
Keep it as a guardrail so this path does not accidentally create Redis
resources while the project uses managed Redis settings for production-style
Kubernetes overlays.

The non-production HA validation package at
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
