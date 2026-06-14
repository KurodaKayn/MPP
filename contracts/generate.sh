#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

(cd "$ROOT" && node --disable-warning=ExperimentalWarning contracts/generate-platform-capabilities.ts)
(cd "$ROOT/backend" && gofmt -w internal/platformcapabilities/capabilities.generated.go)
(cd "$ROOT/frontend" && pnpm generate:contracts && pnpm format src/lib/dashboard/api/generated.ts)
(cd "$ROOT/backend" && go generate ./internal/contracts)
(cd "$ROOT/browser-worker" && go generate ./internal/contracts)
(cd "$ROOT/ai-service" && uv run datamodel-codegen \
  --input ../contracts/views/ai-service.openapi.yaml \
  --input-file-type openapi \
  --output contract_schemas.py \
  --output-model-type pydantic_v2.BaseModel \
  --target-python-version 3.12 \
  --use-standard-collections \
  --use-union-operator \
  --formatters black isort \
  --disable-timestamp && \
  uv run ruff format contract_schemas.py)
