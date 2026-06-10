#!/usr/bin/env bash
set -euo pipefail

cd extension
pnpm install --frozen-lockfile
pnpm run lint
pnpm run format:check
pnpm run compile
pnpm run build
pnpm test:run
