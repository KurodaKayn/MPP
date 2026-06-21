# Production Self-Hosted HA Overlay

This overlay is not an active deployment path for the project.

The checked-in overlay renders only the `production-self-hosted-ha-retired`
ConfigMap. Keep it as a guardrail so this path does not accidentally create
self-hosted production Redis resources while production-style Kubernetes
configuration uses managed Redis settings.

Validation:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/production-self-hosted-ha > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/production-self-hosted-ha \
  "$rendered"
```
