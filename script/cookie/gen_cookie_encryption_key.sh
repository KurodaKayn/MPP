#!/usr/bin/env bash
set -euo pipefail

# COOKIE_ENCRYPTION_KEY is used directly as an AES-256 key, so it must be
# exactly 32 bytes after reading from the environment.
if command -v openssl >/dev/null 2>&1; then
    key="$(openssl rand -base64 24 | tr '+/' '-_')"
elif command -v python3 >/dev/null 2>&1; then
    key="$(
        python3 - <<'PY'
import base64
import os

print(base64.b64encode(os.urandom(24)).decode("ascii").replace("+", "-").replace("/", "_"))
PY
    )"
else
    echo "openssl or python3 is required to generate COOKIE_ENCRYPTION_KEY" >&2
    exit 1
fi

if [ "${#key}" -ne 32 ]; then
    echo "generated COOKIE_ENCRYPTION_KEY must be exactly 32 characters" >&2
    exit 1
fi

printf 'COOKIE_ENCRYPTION_KEY=%s\n' "$key"
