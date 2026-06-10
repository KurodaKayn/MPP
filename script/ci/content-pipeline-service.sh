#!/usr/bin/env bash
set -euo pipefail

cd content-pipeline-service
cargo fmt --all --check
cargo clippy -- -D warnings
cargo test
