#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "$ROOT_DIR"
pnpm install --frozen-lockfile --filter mpp-extension-publisher...
pnpm --dir extension run lint
pnpm --dir extension run format:check
pnpm --dir extension run compile
pnpm --dir extension run build
pnpm --dir extension test:run
