#!/usr/bin/env bash
set -euo pipefail

cd collab-service
pnpm install --frozen-lockfile
pnpm run lint
pnpm run format:check
pnpm run type-check
pnpm test
