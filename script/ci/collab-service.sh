#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${CI_REPORT_DIR:-$ROOT_DIR/artifacts/collab-service}"

cd "$ROOT_DIR"
mkdir -p "$REPORT_DIR"
pnpm install --frozen-lockfile --filter collab-service...
pnpm --dir collab-service run lint
pnpm --dir collab-service run format:check
pnpm --dir collab-service run type-check
pnpm --dir collab-service exec vitest run --reporter=default --reporter=junit --outputFile.junit="$REPORT_DIR/junit.xml"
