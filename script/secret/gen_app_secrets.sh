#!/usr/bin/env bash
set -euo pipefail

kind="${1:-all}"

hex_secret() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
    elif command -v python3 >/dev/null 2>&1; then
        python3 - <<'PY'
import secrets

print(secrets.token_hex(32))
PY
    else
        echo "openssl or python3 is required to generate hex secrets" >&2
        exit 1
    fi
}

cookie_key() {
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
    printf '%s\n' "$key"
}

print_secret() {
    case "$1" in
        jwt)
            printf 'JWT_SECRET=%s\n' "$(hex_secret)"
            ;;
        cookie)
            printf 'COOKIE_ENCRYPTION_KEY=%s\n' "$(cookie_key)"
            ;;
        collab)
            printf 'COLLAB_TOKEN_SECRET=%s\n' "$(hex_secret)"
            ;;
        ai)
            printf 'AI_SERVICE_INTERNAL_TOKEN=%s\n' "$(hex_secret)"
            ;;
        pipeline)
            printf 'CONTENT_PIPELINE_INTERNAL_TOKEN=%s\n' "$(hex_secret)"
            ;;
        browser)
            printf 'BROWSER_WORKER_INTERNAL_TOKEN=%s\n' "$(hex_secret)"
            ;;
        db)
            printf 'DB_PASSWORD=%s\n' "$(hex_secret)"
            ;;
        redis)
            printf 'REDIS_PASSWORD=%s\n' "$(hex_secret)"
            ;;
        grafana)
            printf 'GRAFANA_ADMIN_PASSWORD=%s\n' "$(hex_secret)"
            ;;
        app)
            print_secret jwt
            print_secret cookie
            print_secret collab
            print_secret ai
            print_secret pipeline
            print_secret browser
            ;;
        infra)
            print_secret db
            print_secret redis
            print_secret grafana
            ;;
        all)
            print_secret app
            print_secret infra
            ;;
        *)
            echo "usage: $0 [jwt|cookie|collab|ai|pipeline|browser|db|redis|grafana|app|infra|all]" >&2
            exit 2
            ;;
    esac
}

print_secret "$kind"
