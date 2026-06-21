# Non-Production Redis Cluster

This package deploys the non-production Redis Cluster topology used to validate
the intended production-style shape without changing production Redis:

- Six Redis Pods in one StatefulSet: three masters and three replicas.
- TLS-only Redis traffic on `6379` and Cluster bus traffic on `16379`.
- Password auth from `mpp-app-secrets/REDIS_PASSWORD`.
- TLS material from a pre-created `mpp-redis-cluster-tls` Secret with
  `ca.crt`, `tls.crt`, and `tls.key`.
- A bootstrap Job that runs `redis-cli --cluster create` with one replica per
  master.
- A cluster-aware `redis_exporter` Deployment for per-node and slot metrics.
- A backup CronJob that schedules node snapshots and records Cluster topology.

It intentionally does not replace the existing `redis` Service or switch app
traffic by default. Non-production apps keep using `REDIS_ENDPOINT_MODE=direct`
and `REDIS_ADDR=redis:6379` unless a Cluster cutover is explicitly rehearsed.

## Apply

Create non-production TLS material before applying the overlay:

```bash
kubectl create secret tls mpp-redis-cluster-tls \
  -n "$MPP_APP_NS" \
  --cert=redis-cluster-nonprod.crt \
  --key=redis-cluster-nonprod.key
kubectl patch secret mpp-redis-cluster-tls \
  -n "$MPP_APP_NS" \
  --type=json \
  -p='[{"op":"add","path":"/data/ca.crt","value":"'"$(base64 -w0 redis-cluster-nonprod-ca.crt)"'"}]'
```

Then render and apply a non-production overlay that includes this package:

```bash
kubectl kustomize deploy/kubernetes/overlays/staging-self-hosted > /tmp/mpp-staging.yaml
ruby script/kubernetes/validate-rendered-manifests.rb \
  deploy/kubernetes/overlays/staging-self-hosted \
  /tmp/mpp-staging.yaml
kubectl apply -f /tmp/mpp-staging.yaml
```

## Validation Record

Record every drill in this table before promoting Cluster settings elsewhere.

| Check | Command | Expected result | Observed result |
| --- | --- | --- | --- |
| Rollout | `kubectl rollout status statefulset/redis-cluster -n "$MPP_APP_NS"` | Six Pods ready after bootstrap | |
| Bootstrap | `kubectl logs job/redis-cluster-bootstrap -n "$MPP_APP_NS"` | `cluster_state:ok` or successful cluster creation | |
| Slots | `redis-cli --tls --cacert ca.crt -a "$REDIS_PASSWORD" -h redis-cluster-0.redis-cluster-headless.mpp-system.svc.cluster.local CLUSTER SLOTS` | Slots `0..16383` covered | |
| Failover | `redis-cli --tls --cacert ca.crt -a "$REDIS_PASSWORD" -h <replica> CLUSTER FAILOVER` | Replica promoted and clients recover | |
| Reshard | `redis-cli --tls --cacert ca.crt -a "$REDIS_PASSWORD" --cluster reshard <node>:6379` | Slots move without uncovered slots | |
| Backup | `kubectl create job --from=cronjob/redis-cluster-backup redis-cluster-backup-manual -n "$MPP_APP_NS"` | Backup marker and topology files created | |
| Restore | Restore RDB/AOF data into a fresh non-prod Cluster and run `CLUSTER INFO` | `cluster_state:ok`; sampled app keys read back | |
| Metrics | Scrape `redis-cluster-exporter:9121/metrics` | Per-node, memory, command, keyspace, and Cluster metrics visible | |
| Hot keys | `redis-cli --tls --cacert ca.crt -a "$REDIS_PASSWORD" --hotkeys -h <node>` | Hot-key sample recorded or provider gap noted | |

Capture failover and resharding timings with:

```bash
start="$(date +%s)"
# run failover or reshard command here
until redis-cli --tls --cacert ca.crt -a "$REDIS_PASSWORD" -h "$REDIS_CLUSTER_HOST" CLUSTER INFO \
  | tr -d '\r' | grep -q '^cluster_state:ok$'; do
  sleep 2
done
echo "Redis Cluster recovered in $(($(date +%s) - start))s"
```

## Application Cutover Rehearsal

Only use Cluster mode in non-production after the validation record is complete:

```yaml
REDIS_ENDPOINT_MODE: cluster
REDIS_ADDR: redis-cluster.mpp-system.svc.cluster.local:6379
REDIS_TLS: "true"
```

To roll back app traffic, patch the direct-mode values and restart the
non-production app Deployments:

```yaml
REDIS_ENDPOINT_MODE: direct
REDIS_ADDR: redis:6379
REDIS_TLS: "false"
```

## Metrics

The exporter is configured with `REDIS_EXPORTER_IS_CLUSTER=true` and TLS client
material. Use the metrics Service for Prometheus discovery or a port-forward:

```bash
kubectl port-forward -n "$MPP_APP_NS" svc/redis-cluster-exporter 9121:9121
curl -fsS http://127.0.0.1:9121/metrics | grep -E 'redis_(cluster|memory|keyspace|commands|up)'
```

If a provider-managed Cluster is selected later and hides per-slot, failover, or
hot-key metrics, record that as a provider gap in the validation table.

## Teardown

Teardown must only target non-production names:

```bash
kubectl delete -n "$MPP_APP_NS" job redis-cluster-bootstrap --ignore-not-found
kubectl delete -k deploy/kubernetes/data-services/redis-cluster-nonprod
kubectl delete -n "$MPP_APP_NS" pvc -l app.kubernetes.io/component=redis-cluster
kubectl delete -n "$MPP_APP_NS" secret mpp-redis-cluster-tls --ignore-not-found
```

Do not delete the `redis` Service, production managed Redis settings, or any
Secret outside the non-production namespace.
