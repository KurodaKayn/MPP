#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GRPC_ADDR="${CONTENT_PIPELINE_INTEGRATION_GRPC_ADDR:-127.0.0.1:55051}"
METRICS_ADDR="${CONTENT_PIPELINE_INTEGRATION_METRICS_ADDR:-127.0.0.1:59090}"
LOG_FILE="${CONTENT_PIPELINE_INTEGRATION_LOG_FILE:-$(mktemp)}"

if [[ "$GRPC_ADDR" != 127.0.0.1:* ]]; then
  echo "CONTENT_PIPELINE_INTEGRATION_GRPC_ADDR must use 127.0.0.1:<port>" >&2
  exit 1
fi

GRPC_HOST="${GRPC_ADDR%:*}"
GRPC_PORT="${GRPC_ADDR##*:}"

cleanup() {
  if [[ -n "${SERVICE_PID:-}" ]] && kill -0 "$SERVICE_PID" 2>/dev/null; then
    kill "$SERVICE_PID" 2>/dev/null || true
    wait "$SERVICE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

(
  cd "$ROOT_DIR/content-pipeline-service"
  cargo build -p content-pipeline-service
)

(
  CONTENT_PIPELINE_ADDR="$GRPC_ADDR" \
    CONTENT_PIPELINE_METRICS_ADDR="$METRICS_ADDR" \
    "$ROOT_DIR/content-pipeline-service/target/debug/content-pipeline-service"
) >"$LOG_FILE" 2>&1 &
SERVICE_PID=$!

echo "content-pipeline-service log: $LOG_FILE"

for _ in $(seq 1 90); do
  if ! kill -0 "$SERVICE_PID" 2>/dev/null; then
    echo "content-pipeline-service exited before becoming ready" >&2
    tail -200 "$LOG_FILE" >&2 || true
    exit 1
  fi
  if (echo >"/dev/tcp/$GRPC_HOST/$GRPC_PORT") >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! (echo >"/dev/tcp/$GRPC_HOST/$GRPC_PORT") >/dev/null 2>&1; then
  echo "content-pipeline-service did not open $GRPC_ADDR" >&2
  tail -200 "$LOG_FILE" >&2 || true
  exit 1
fi

(
  cd "$ROOT_DIR/backend"
  CONTENT_PIPELINE_HOST="$GRPC_HOST" \
    CONTENT_PIPELINE_PORT="$GRPC_PORT" \
    go test -tags=contentpipeline_integration ./internal/pkg/media ./internal/services/compiler
)
