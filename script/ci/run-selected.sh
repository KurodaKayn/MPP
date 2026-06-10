#!/usr/bin/env bash
set -euo pipefail

ran_any=false

run_if_selected() {
  local selected="$1"
  local name="$2"
  local script="$3"

  if [[ "$selected" != "true" ]]; then
    echo "Skipping $name."
    return
  fi

  ran_any=true
  echo "::group::$name"
  bash "$script"
  echo "::endgroup::"
}

run_if_selected "${CI_FRONTEND:-false}" "Frontend" script/ci/frontend.sh
run_if_selected "${CI_EXTENSION:-false}" "Extension" script/ci/extension.sh
run_if_selected "${CI_BACKEND:-false}" "Backend" script/ci/backend.sh
run_if_selected "${CI_BROWSER_WORKER:-false}" "Browser worker" script/ci/browser-worker.sh
run_if_selected "${CI_AI_SERVICE:-false}" "AI service" script/ci/ai-service.sh
run_if_selected "${CI_COLLAB_SERVICE:-false}" "Collab service" script/ci/collab-service.sh
run_if_selected "${CI_CONTENT_PIPELINE_SERVICE:-false}" "Content pipeline service" script/ci/content-pipeline-service.sh
run_if_selected "${CI_KUBERNETES:-false}" "Kubernetes manifests" script/ci/kubernetes.sh

if [[ "$ran_any" != "true" ]]; then
  echo "No CI areas selected."
fi
