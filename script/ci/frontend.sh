#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "$ROOT_DIR"
pnpm install --frozen-lockfile --filter frontend...
pnpm --dir frontend run lint
pnpm --dir frontend run format:check
pnpm --dir frontend run type-check
pnpm --dir frontend run build
pnpm --dir frontend test
