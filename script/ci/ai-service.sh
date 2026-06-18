#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${CI_REPORT_DIR:-$ROOT_DIR/artifacts/ai-service}"

mkdir -p "$REPORT_DIR"

cd "$ROOT_DIR/ai-service"
uv sync --locked
uv run ruff check .
uv run ruff format --check .
uv run pytest --junit-xml="$REPORT_DIR/junit.xml"
