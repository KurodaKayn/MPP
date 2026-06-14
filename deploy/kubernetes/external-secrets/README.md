# External Secrets

This package materializes the `mpp-app-secrets` Kubernetes Secret through
External Secrets Operator. It is provider-neutral: the checked-in
`ExternalSecret` references a placeholder `ClusterSecretStore` named
`mpp-production-secrets` and placeholder remote keys under `mpp/production/*`.

Required overlay inputs:

- Install External Secrets Operator and its CRDs in the target cluster.
- Create a provider-specific `ClusterSecretStore` named `mpp-production-secrets`
  or patch `spec.secretStoreRef` to the store used by the environment.
- Replace every `spec.data[*].remoteRef.key` with the real provider key path.
- Add a `REDIS_PASSWORD` mapping when the managed Redis provider requires auth.

The package must not contain a raw Kubernetes `Secret`. The operator owns the
runtime `mpp-app-secrets` Secret and keeps the value contract explicit through
one `spec.data` entry per required app key.

Render and validate:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/external-secrets > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/external-secrets \
  "$rendered"
```
