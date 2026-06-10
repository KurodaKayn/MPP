#!/usr/bin/env bash
set -euo pipefail

cd frontend
pnpm install --frozen-lockfile
pnpm run lint
pnpm run type-check
pnpm test
