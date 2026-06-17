#!/usr/bin/env bash
set -euo pipefail

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

cd script/kubernetes/smoke-test
go test ./...
smoke_report_dir="$(mktemp -d)"
go run . \
  --dry-run \
  --skip-public \
  --report-json "$smoke_report_dir/smoke-report.json" \
  --report-junit "$smoke_report_dir/smoke-junit.xml" \
  > "$smoke_report_dir/smoke.log"
test -s "$smoke_report_dir/smoke-report.json"
test -s "$smoke_report_dir/smoke-junit.xml"
