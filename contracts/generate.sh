#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

(cd "$ROOT/frontend" && pnpm generate:contracts)
(cd "$ROOT/backend" && go generate ./internal/contracts)
(cd "$ROOT/ai-service" && uv run datamodel-codegen \
  --input ../contracts/openapi.yaml \
  --input-file-type openapi \
  --output contract_schemas.py \
  --output-model-type pydantic_v2.BaseModel \
  --target-python-version 3.12 \
  --use-standard-collections \
  --use-union-operator \
  --disable-timestamp)
