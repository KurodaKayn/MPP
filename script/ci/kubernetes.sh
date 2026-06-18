#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${CI_REPORT_DIR:-$ROOT_DIR/artifacts/kubernetes}"

mkdir -p "$REPORT_DIR"
cd "$ROOT_DIR"

find deploy/kubernetes -name kustomization.yaml -print | sort | while IFS= read -r package; do
  dir="$(dirname "$package")"
  rendered="$(mktemp)"
  echo "::group::kubectl kustomize $dir"
  kubectl kustomize "$dir" > "$rendered"
  ruby script/kubernetes/validate-rendered-manifests.rb "$dir" "$rendered"
  ruby script/kubernetes/validate-rendered-schema.rb "$dir" "$rendered"
  echo "::endgroup::"
done

ruby script/env/generate_examples.rb --check
ruby script/kubernetes/test_app_secret_materializer.rb
ruby script/kubernetes/test_overlay_image_pinner.rb
ruby script/kubernetes/test_redis_ha_failover_drill.rb
ruby script/kubernetes/test_redis_ha_migration_rehearsal.rb
ruby script/kubernetes/test_redis_capacity_alerts.rb
ruby script/kubernetes/test_validate_rendered_manifests.rb
ruby script/kubernetes/test_validate_rendered_schema.rb
ruby script/redis/test_keyspace_inventory.rb

cd "$ROOT_DIR/script/kubernetes/smoke-test"
go test ./...
go run . \
  --dry-run \
  --skip-public \
  --report-json "$REPORT_DIR/smoke-report.json" \
  --report-junit "$REPORT_DIR/smoke-junit.xml" \
  > "$REPORT_DIR/smoke.log"
test -s "$REPORT_DIR/smoke-report.json"
test -s "$REPORT_DIR/smoke-junit.xml"
