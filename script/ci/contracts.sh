#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

generated_files=(
  "frontend/src/lib/dashboard/api/generated.ts"
  "backend/internal/contracts/openapi.gen.go"
  "browser-worker/internal/contracts/openapi.gen.go"
  "ai-service/contract_schemas.py"
)

cd "$ROOT_DIR"

(cd frontend && pnpm install --frozen-lockfile)
(cd ai-service && uv sync --locked)

sh contracts/generate.sh

if ! git diff --quiet -- "${generated_files[@]}"; then
  echo "Generated contract files are out of date. Run: sh contracts/generate.sh" >&2
  git diff -- "${generated_files[@]}" >&2
  exit 1
fi
