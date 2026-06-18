#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${CI_REPORT_DIR:-$ROOT_DIR/artifacts/frontend}"

cd "$ROOT_DIR"
mkdir -p "$REPORT_DIR"
pnpm install --frozen-lockfile --filter frontend...
pnpm --dir frontend run lint
pnpm --dir frontend run format:check
pnpm --dir frontend run type-check
pnpm --dir frontend run build
pnpm --dir frontend exec vitest run --reporter=default --reporter=junit --outputFile.junit="$REPORT_DIR/junit.xml"
