#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

cd "$ROOT_DIR"

ACTION="${1:-status}"
FRONTEND_CONTAINER="mpp-dev-frontend-1"
FRONTEND_NEXT_VOLUME="mpp-dev_frontend_next"
HELPER_IMAGE="${DOCKER_CACHE_HELPER_IMAGE:-node:24-alpine}"

compose() {
  env_file_args=""
  if [ -f docker/.env ]; then
    env_file_args="--env-file docker/.env"
  fi

  # shellcheck disable=SC2086
  docker compose $env_file_args \
    -f docker/docker-compose.yml \
    -f docker/docker-compose.dev.yml \
    "$@"
}

volume_exists() {
  docker volume inspect "$FRONTEND_NEXT_VOLUME" >/dev/null 2>&1
}

frontend_running() {
  docker container inspect "$FRONTEND_CONTAINER" \
    --format '{{.State.Running}}' 2>/dev/null | grep -q '^true$'
}

run_helper() {
  mode="$1"
  command="$2"

  docker run --rm \
    -v "$FRONTEND_NEXT_VOLUME:/cache:$mode" \
    "$HELPER_IMAGE" \
    sh -lc "$command"
}

status_frontend_next() {
  if ! volume_exists; then
    printf 'Volume %s does not exist.\n' "$FRONTEND_NEXT_VOLUME"
    return
  fi

  run_helper ro '
    printf "Top-level .next volume usage:\n"
    du -h -d 2 /cache
    printf "\nTurbopack cache usage:\n"
    du -h -d 3 /cache/dev/cache 2>/dev/null || true
  '
}

clean_frontend_next_cache() {
  if ! volume_exists; then
    printf 'Volume %s does not exist; nothing to clean.\n' "$FRONTEND_NEXT_VOLUME"
    return
  fi

  was_running=false
  if frontend_running; then
    was_running=true
    compose stop frontend
  fi

  run_helper rw '
    rm -rf /cache/dev/cache /cache/cache
    mkdir -p /cache/dev/cache
  '

  if [ "$was_running" = true ]; then
    compose up -d frontend
  fi

  status_frontend_next
}

reset_frontend_next_volume() {
  compose rm -f -s frontend

  if volume_exists; then
    docker volume rm "$FRONTEND_NEXT_VOLUME"
  fi

  compose up -d frontend
}

case "$ACTION" in
  status)
    status_frontend_next
    ;;
  clean-frontend-next)
    clean_frontend_next_cache
    ;;
  reset-frontend-next)
    reset_frontend_next_volume
    ;;
  *)
    printf 'Usage: %s [status|clean-frontend-next|reset-frontend-next]\n' "$0" >&2
    exit 2
    ;;
esac
