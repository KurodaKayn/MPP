# Kubernetes Observability

This package adds Kubernetes-native log and metric discovery for MPP. It is an
optional base for environments that run Grafana Alloy, Loki, and a
Prometheus-Operator-compatible metrics stack.

It provides:

- Alloy Pod discovery for `mpp-system` and `mpp-browser-runtime` logs.
- PodMonitor resources for HTTP service metrics, content-pipeline metrics, and
  Alloy self metrics.
- PrometheusRule alerts for browser runtime startup failures, cleanup failures,
  cleanup lag, service readiness failures, Redis-dependent readiness failures,
  and publish-worker job failures.

Required overlay inputs:

- Patch `LOKI_WRITE_URL` when Loki is outside `mpp-observability`.
- Install the Prometheus Operator CRDs before applying the PodMonitor and
  PrometheusRule resources.
- Run Prometheus in `mpp-observability`, or label the Prometheus namespace with
  `mpp.kurodakayn.dev/metrics-scraper=true` so it can scrape browser-worker.
- Add this package to the same environment overlay that deploys
  `browser-runtime-control` and `app-baseline`.

The Alloy configuration keeps Docker discovery out of Kubernetes deployments and
uses Pod labels for `service`, `namespace`, `pod`, `container`, `platform`, and
runtime driver metadata.
