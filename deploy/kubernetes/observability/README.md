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
  `mpp.kurodakayn.dev/metrics-scraper=true`. That label allows scraping app
  metrics for backend and publish-worker (`8080`), browser-worker (`8081`),
  ai-service (`8000`), collab-service (`8090`), and content-pipeline (`9090`).
  Treat that namespace label as a trusted metrics boundary: Kubernetes
  NetworkPolicy is layer 4 only, so shared HTTP listener targets allow the
  labeled namespace to reach the full service port rather than only `/metrics`.
  Content-pipeline uses a dedicated metrics listener on `9090`.
- Add this package to the same environment overlay that deploys
  `browser-runtime-control` and `app-baseline`.

The Alloy configuration keeps Docker discovery out of Kubernetes deployments and
uses Pod labels for `service`, `namespace`, `pod`, `container`, `platform`, and
runtime driver metadata.
