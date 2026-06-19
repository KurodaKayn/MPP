# Retired Production Self-Hosted HA Overlay

This overlay was the Phase 2 production endpoint-switch overlay for moving from
the original self-hosted Redis StatefulSet to self-hosted HA Redis. It is
retired by issue #339 after the production managed Redis soak completed.

Do not apply this overlay to production to recreate self-hosted Redis. The only
rendered object is the `production-self-hosted-ha-retired` ConfigMap, which
records that `deploy/kubernetes/overlays/production-managed` is the active
production Redis path.

Decommission evidence and rollback notes live in
`doc/self-hosted-redis-decommission-record.md`.

To reconstruct the old deployment during the retention window, use the Git SHA
recorded in the decommission record, render this overlay from that historical
revision, restore the retained self-hosted Redis snapshot into the recreated HA
Redis primary, then switch application traffic back to Sentinel mode only after
the rollback owner approves the restore.

Validation:

```bash
rendered="$(mktemp)"
kubectl kustomize deploy/kubernetes/overlays/production-self-hosted-ha > "$rendered"
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/production-self-hosted-ha \
  "$rendered"
```
