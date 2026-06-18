#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${CI_REPORT_DIR:-$ROOT_DIR/artifacts/extension}"

cd "$ROOT_DIR"
mkdir -p "$REPORT_DIR"
pnpm install --frozen-lockfile --filter mpp-extension-publisher...
pnpm --dir extension run lint
pnpm --dir extension run format:check
pnpm --dir extension run compile
pnpm --dir extension run build
pnpm --dir extension exec vitest run --reporter=default --reporter=junit --outputFile.junit="$REPORT_DIR/junit.xml"
