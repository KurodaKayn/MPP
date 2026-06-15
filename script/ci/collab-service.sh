#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "$ROOT_DIR"
pnpm install --frozen-lockfile --filter collab-service...
pnpm --dir collab-service run lint
pnpm --dir collab-service run format:check
pnpm --dir collab-service run type-check
pnpm --dir collab-service test
