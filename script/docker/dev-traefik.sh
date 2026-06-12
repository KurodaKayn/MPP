#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

cd "$ROOT_DIR"

ACTION="${1:-up}"

export COMPOSE_PROFILES="${COMPOSE_PROFILES:-gateway}"

ensure_env_file() {
  cp -n deploy/docker/.env.dev.example deploy/docker/.env
}

env_value() {
  name="$1"
  default="$2"
  eval "value=\${$name:-}"
  if [ -n "$value" ]; then
    printf '%s\n' "$value"
    return
  fi

  value=$(
    awk -F= -v key="$name" '$1 == key { print substr($0, length(key) + 2); exit }' deploy/docker/.env 2>/dev/null || true
  )
  if [ -n "$value" ]; then
    printf '%s\n' "$value"
    return
  fi

  printf '%s\n' "$default"
}

compose() {
  ensure_env_file
  export TRAEFIK_HTTP_PORT="$(env_value TRAEFIK_HTTP_PORT 8088)"
  export TRAEFIK_HTTPS_PORT="$(env_value TRAEFIK_HTTPS_PORT 8443)"
  docker compose \
    --env-file deploy/docker/.env \
    -f deploy/docker/docker-compose.yml \
    -f deploy/docker/docker-compose.dev.yml \
    --profile gateway \
    "$@"
}

case "$ACTION" in
  up|start)
    compose up -d --no-deps traefik
    printf 'Traefik dev gateway: http://localhost:%s\n' "$TRAEFIK_HTTP_PORT"
    printf 'Traefik dev HTTPS: https://localhost:%s\n' "$TRAEFIK_HTTPS_PORT"
    ;;
  restart)
    compose up -d --no-deps --force-recreate traefik
    ;;
  stop)
    compose stop traefik
    ;;
  rm|remove)
    compose rm -f -s traefik
    ;;
  logs)
    compose logs -f traefik
    ;;
  ps|status)
    compose ps traefik || true
    ;;
  *)
    printf 'Usage: %s [up|restart|stop|rm|logs|ps]\n' "$0" >&2
    exit 2
    ;;
esac
