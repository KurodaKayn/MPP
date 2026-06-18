#!/usr/bin/env bash
set -euo pipefail

cd browser-worker
go test ./...
