#!/usr/bin/env bash
set -euo pipefail

cd ai-service
uv sync --locked
uv run ruff check .
uv run ruff format --check .
uv run pytest
