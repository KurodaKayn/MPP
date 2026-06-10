#!/usr/bin/env bash
set -euo pipefail

event_name="${1:-}"
base_sha="${2:-}"
head_sha="${3:-HEAD}"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"

write_output() {
  echo "$1=$2" >> "$output_file"
}

mark_all() {
  write_output frontend true
  write_output extension true
  write_output backend true
  write_output browser_worker true
  write_output ai_service true
  write_output collab_service true
  write_output content_pipeline_service true
  write_output kubernetes true
  write_output node true
  write_output go true
}

if [[ "$event_name" != "pull_request" ]]; then
  mark_all
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

mark() {
  local name="$1"
  local pattern="$2"

  if has_changes "$pattern"; then
    write_output "$name" true
  else
    write_output "$name" false
  fi
}

shared_pattern='(\.github/|script/ci/)'
mark frontend "^(frontend/|${shared_pattern})"
mark extension "^(extension/|${shared_pattern})"
mark backend "^(backend/|${shared_pattern})"
mark browser_worker "^(browser-worker/|${shared_pattern})"
mark ai_service "^(ai-service/|${shared_pattern})"
mark collab_service "^(collab-service/|${shared_pattern})"
mark content_pipeline_service "^(content-pipeline-service/|${shared_pattern})"
mark kubernetes "^(deploy/kubernetes/|script/kubernetes/|script/env/|script/secret/|contracts/env\.schema\.yaml|${shared_pattern})"

if has_changes "^(frontend/|extension/|collab-service/|${shared_pattern})"; then
  write_output node true
else
  write_output node false
fi

if has_changes "^(backend/|browser-worker/|deploy/kubernetes/|script/kubernetes/|script/env/|script/secret/|contracts/env\.schema\.yaml|${shared_pattern})"; then
  write_output go true
else
  write_output go false
fi
