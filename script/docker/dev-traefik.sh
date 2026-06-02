#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

cd "$ROOT_DIR"

ACTION="${1:-up}"

export COMPOSE_PROFILES="${COMPOSE_PROFILES:-gateway}"
export TRAEFIK_HTTP_PORT="${TRAEFIK_HTTP_PORT:-8088}"
export TRAEFIK_HTTPS_PORT="${TRAEFIK_HTTPS_PORT:-8443}"

compose() {
  docker compose \
    --env-file docker/.env.dev.example \
    -f docker/docker-compose.yml \
    -f docker/docker-compose.dev.yml \
    --profile gateway \
    "$@"
}

ensure_env_file() {
  cp -n docker/.env.dev.example docker/.env
}

case "$ACTION" in
  up|start)
    ensure_env_file
    compose up -d --no-deps traefik
    printf 'Traefik dev gateway: http://localhost:%s\n' "$TRAEFIK_HTTP_PORT"
    printf 'Traefik dev HTTPS: https://localhost:%s\n' "$TRAEFIK_HTTPS_PORT"
    ;;
  restart)
    ensure_env_file
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
    compose ps traefik
    ;;
  *)
    printf 'Usage: %s [up|restart|stop|rm|logs|ps]\n' "$0" >&2
    exit 2
    ;;
esac
