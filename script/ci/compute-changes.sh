#!/usr/bin/env bash
set -euo pipefail

event_name="${1:-}"
base_sha="${2:-}"
head_sha="${3:-HEAD}"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"

areas=(
  contracts
  frontend
  extension
  backend
  browser_worker
  ai_service
  collab_service
  content_pipeline_service
  content_pipeline_integration
  kubernetes
)

if [[ -z "$event_name" ]]; then
  echo "GitHub event name is required." >&2
  exit 1
fi

for area in "${areas[@]}"; do
  area_output="$(mktemp)"
  trap 'rm -f "$area_output"' EXIT

  GITHUB_OUTPUT="$area_output" \
    bash "$(dirname "${BASH_SOURCE[0]}")/select-area.sh" "$area" "$event_name" "$base_sha" "$head_sha"

  selected="$(sed -n 's/^selected=//p' "$area_output")"
  if [[ -z "$selected" ]]; then
    echo "Failed to compute selection for area: $area" >&2
    exit 1
  fi

  echo "${area}=${selected}" >> "$output_file"
  rm -f "$area_output"
done
