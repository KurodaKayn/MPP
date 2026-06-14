#!/usr/bin/env bash
set -euo pipefail

area="${1:-}"
event_name="${2:-}"
base_sha="${3:-}"
head_sha="${4:-HEAD}"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"

select_area() {
  echo "selected=$1" >> "$output_file"
  echo "$area selected: $1"
}

if [[ -z "$area" ]]; then
  echo "CI area is required." >&2
  exit 1
fi

if [[ "$event_name" != "pull_request" ]]; then
  select_area true
  exit 0
fi

if [[ -z "$base_sha" ]]; then
  echo "PR base SHA is required for pull_request change detection." >&2
  exit 1
fi

changed_files="$(mktemp)"
trap 'rm -f "$changed_files"' EXIT

git diff --name-only "$base_sha" "$head_sha" > "$changed_files"
echo "Changed files:"
cat "$changed_files"

has_changes() {
  grep -Eq "$1" "$changed_files"
}

shared_pattern='(\.github/workflows/ci\.yml|script/ci/)'
contract_pattern='(contracts/|frontend/src/lib/dashboard/api/generated\.ts|backend/internal/contracts/openapi\.gen\.go|browser-worker/internal/contracts/openapi\.gen\.go|ai-service/contract_schemas\.py)'

case "$area" in
  contracts)
    pattern="^(${contract_pattern}|frontend/package\.json|frontend/pnpm-lock\.yaml|backend/go\.mod|backend/go\.sum|browser-worker/go\.mod|browser-worker/go\.sum|ai-service/pyproject\.toml|ai-service/uv\.lock|${shared_pattern})"
    ;;
  frontend)
    pattern="^(frontend/|contracts/|${shared_pattern})"
    ;;
  extension)
    pattern="^(extension/|${shared_pattern})"
    ;;
  backend)
    pattern="^(backend/|contracts/|${shared_pattern})"
    ;;
  browser_worker)
    pattern="^(browser-worker/|contracts/|${shared_pattern})"
    ;;
  ai_service)
    pattern="^(ai-service/|contracts/|${shared_pattern})"
    ;;
  collab_service)
    pattern="^(collab-service/|${shared_pattern})"
    ;;
  content_pipeline_service)
    pattern="^(content-pipeline-service/|${shared_pattern})"
    ;;
  kubernetes)
    pattern="^(deploy/kubernetes/|script/kubernetes/|script/env/|script/secret/|contracts/env\.schema\.yaml|${shared_pattern})"
    ;;
  *)
    echo "Unknown CI area: $area" >&2
    exit 1
    ;;
esac

if has_changes "$pattern"; then
  select_area true
else
  select_area false
fi
