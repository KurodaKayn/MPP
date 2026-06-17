#!/usr/bin/env bash
set -euo pipefail

APP_NS="${MPP_APP_NS:-mpp-system}"
MASTER_NAME="${MPP_REDIS_SENTINEL_MASTER_NAME:-mpp-redis-ha}"
TARGET_SECONDS="${MPP_REDIS_FAILOVER_TARGET_SECONDS:-300}"
POLL_SECONDS="${MPP_REDIS_FAILOVER_POLL_SECONDS:-5}"
REQUEST_TIMEOUT="${MPP_REDIS_FAILOVER_REQUEST_TIMEOUT:-10}"
DRILL_ID="${MPP_REDIS_FAILOVER_DRILL_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"

last_master=""
last_ready_detail=""
last_write_detail=""
backend_pods=""
publish_worker_pods=""
browser_worker_pods=""
backend_probe_pod=""

usage() {
  cat <<USAGE
Usage: MPP_APP_NS=mpp-system $0

Runs the non-production HA Redis client failover drill.

Environment:
  MPP_APP_NS                             Kubernetes app namespace. Default: mpp-system
  MPP_REDIS_SENTINEL_MASTER_NAME         Sentinel master name. Default: mpp-redis-ha
  MPP_REDIS_FAILOVER_TARGET_SECONDS      Maximum recovery window. Default: 300
  MPP_REDIS_FAILOVER_POLL_SECONDS        Probe interval. Default: 5
  MPP_REDIS_FAILOVER_REQUEST_TIMEOUT     In-Pod HTTP probe timeout. Default: 10
  MPP_REDIS_FAILOVER_DRILL_ID            Optional stable drill id for probe emails
USAGE
}

log() {
  printf '[redis-ha-failover-drill] %s\n' "$*"
}

fail() {
  printf '[redis-ha-failover-drill] ERROR: %s\n' "$*" >&2
  diagnostics >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

require_can_i() {
  local verb="$1"
  local resource="$2"
  local answer

  answer="$(kubectl auth can-i "$verb" "$resource" -n "$APP_NS" 2>/dev/null || true)"
  [[ "$answer" == "yes" ]] || fail "kubectl user cannot $verb $resource in namespace $APP_NS"
}

config_value() {
  kubectl get configmap mpp-app-config -n "$APP_NS" -o "jsonpath={.data.$1}" 2>/dev/null || true
}

kubectl_allow_failure() {
  kubectl "$@" 2>&1 || true
}

sentinel_exec() {
  kubectl exec -n "$APP_NS" statefulset/redis-ha-sentinel -c sentinel -- \
    env REDIS_SENTINEL_MASTER_NAME="$MASTER_NAME" sh -ec "$1"
}

select_backend_pods() {
  select_pods 'app.kubernetes.io/name=mpp,app.kubernetes.io/component=backend'
}

select_publish_worker_pods() {
  select_pods 'app.kubernetes.io/name=mpp,app.kubernetes.io/component=publish-worker'
}

select_browser_worker_pods() {
  select_pods 'app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-worker'
}

select_pods() {
  local selector="$1"

  kubectl get pod -n "$APP_NS" \
    -l "$selector" \
    -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{" "}{end}'
}

sentinel_master() {
  sentinel_exec 'redis-cli -p 26379 --raw SENTINEL get-master-addr-by-name "$REDIS_SENTINEL_MASTER_NAME" | tr "\n" " "; echo'
}

trigger_sentinel_failover() {
  sentinel_exec 'redis-cli -p 26379 SENTINEL failover "$REDIS_SENTINEL_MASTER_NAME"'
}

workload_ready_probe() {
  local pods="$1"
  local container="$2"
  local url="$3"
  local detail
  local pod
  local ok=0

  for pod in $pods; do
    detail="$(http_ready_probe "$pod" "$container" "$url")"
    append_ready_detail "$container/$pod $detail"
    if [[ "$detail" != exit=0* || "$detail" != *'"status":"ready"'* ]]; then
      ok=1
    fi
  done

  return "$ok"
}

append_ready_detail() {
  local detail="$1"

  if [[ -n "$last_ready_detail" ]]; then
    last_ready_detail="${last_ready_detail}; ${detail}"
  else
    last_ready_detail="$detail"
  fi
}

backend_ready_probe() {
  workload_ready_probe "$backend_pods" backend http://127.0.0.1:8080/ready
}

publish_worker_ready_probe() {
  workload_ready_probe "$publish_worker_pods" publish-worker http://127.0.0.1:8080/ready
}

browser_worker_ready_probe() {
  workload_ready_probe "$browser_worker_pods" browser-worker http://127.0.0.1:8081/ready
}

app_clients_ready_probe() {
  local ok=0

  last_ready_detail=""
  backend_ready_probe || ok=1
  publish_worker_ready_probe || ok=1
  browser_worker_ready_probe || ok=1
  return "$ok"
}

http_ready_probe() {
  local pod="$1"
  local container="$2"
  local url="$3"
  local output
  local status

  set +e
  output="$(
    kubectl exec -n "$APP_NS" "$pod" -c "$container" -- \
      wget -qO- -T "$REQUEST_TIMEOUT" "$url" 2>&1
  )"
  status=$?
  set -e

  printf 'exit=%s output=%s' "$status" "$(one_line "$output")"
}

backend_write_probe() {
  local attempt="$1"
  local email="redis-ha-drill-${DRILL_ID}-${attempt}@example.invalid"
  local payload
  local read_output
  local read_status
  local write_output
  local write_status

  payload="$(printf '{"email":"%s","scene":"register"}' "$email")"
  set +e
  write_output="$(
    kubectl exec -n "$APP_NS" "$backend_probe_pod" -c backend -- \
      sh -ec '
        payload="$1"
        wget -qO- -T "'"$REQUEST_TIMEOUT"'" \
          --header="Content-Type: application/json" \
          --post-data="$payload" \
          http://127.0.0.1:8080/api/auth/send-code
      ' sh "$payload" 2>&1
  )"
  write_status=$?
  read_output="$(
    kubectl exec -n "$APP_NS" "$backend_probe_pod" -c backend -- \
      sh -ec '
        payload="$1"
        wget -qO- -T "'"$REQUEST_TIMEOUT"'" \
          --header="Content-Type: application/json" \
          --post-data="$payload" \
          http://127.0.0.1:8080/api/auth/send-code
      ' sh "$payload" 2>&1
  )"
  read_status=$?
  set -e

  last_write_detail="email=$email write_exit=$write_status write_output=$(one_line "$write_output") read_exit=$read_status read_output=$(one_line "$read_output")"
  [[ $write_status -eq 0 && "$write_output" == *'verification code sent'* && "$read_output" == *'rate_limited'* ]]
}

pod_env_value() {
  local pod="$1"
  local container="$2"
  local env_name="$3"
  kubectl exec -n "$APP_NS" "$pod" -c "$container" -- sh -ec "printf '%s' \"\${$env_name:-}\""
}

pod_redis_mode_ok() {
  local pod="$1"
  local container="$2"
  local label="$3"

  [[ "$(pod_env_value "$pod" "$container" REDIS_ENDPOINT_MODE)" == "sentinel" ]] ||
    fail "$label is not running with REDIS_ENDPOINT_MODE=sentinel; restart it after patching mpp-app-config"
  [[ "$(pod_env_value "$pod" "$container" REDIS_SENTINEL_ADDRS)" == "redis-ha-sentinel:26379" ]] ||
    fail "$label is not running with REDIS_SENTINEL_ADDRS=redis-ha-sentinel:26379; restart it after patching mpp-app-config"
  [[ "$(pod_env_value "$pod" "$container" REDIS_SENTINEL_MASTER_NAME)" == "$MASTER_NAME" ]] ||
    fail "$label is not running with REDIS_SENTINEL_MASTER_NAME=$MASTER_NAME; restart it after patching mpp-app-config"
  [[ "$(pod_env_value "$pod" "$container" REDIS_ADDR)" == "redis:6379" ]] ||
    fail "$label must keep REDIS_ADDR=redis:6379 for rollback"
}

workload_redis_mode_ok() {
  local pods="$1"
  local container="$2"
  local label="$3"
  local pod

  for pod in $pods; do
    pod_redis_mode_ok "$pod" "$container" "$label Pod $pod"
  done
}

one_line() {
  printf '%s' "$1" | tr '\n' ' ' | sed 's/[[:space:]][[:space:]]*/ /g'
}

validate_nonprod_config() {
  local app_env
  local endpoint_mode
  local sentinel_addrs
  local sentinel_master
  local redis_addr

  app_env="$(config_value APP_ENV)"
  endpoint_mode="$(config_value REDIS_ENDPOINT_MODE)"
  sentinel_addrs="$(config_value REDIS_SENTINEL_ADDRS)"
  sentinel_master="$(config_value REDIS_SENTINEL_MASTER_NAME)"
  redis_addr="$(config_value REDIS_ADDR)"

  case "$app_env" in
    production|prod)
      fail "refusing to run against APP_ENV=$app_env"
      ;;
    "")
      fail "mpp-app-config APP_ENV must be set before running this drill"
      ;;
  esac

  [[ "$endpoint_mode" == "sentinel" ]] ||
    fail "mpp-app-config REDIS_ENDPOINT_MODE must be sentinel; got ${endpoint_mode:-<empty>}"
  [[ "$sentinel_addrs" == "redis-ha-sentinel:26379" ]] ||
    fail "mpp-app-config REDIS_SENTINEL_ADDRS must be redis-ha-sentinel:26379; got ${sentinel_addrs:-<empty>}"
  [[ "${sentinel_master:-$MASTER_NAME}" == "$MASTER_NAME" ]] ||
    fail "mpp-app-config REDIS_SENTINEL_MASTER_NAME must be $MASTER_NAME; got $sentinel_master"
  [[ "$redis_addr" == "redis:6379" ]] ||
    fail "mpp-app-config REDIS_ADDR must stay redis:6379 for direct rollback; got ${redis_addr:-<empty>}"

  log "config ok: APP_ENV=$app_env REDIS_ENDPOINT_MODE=$endpoint_mode REDIS_SENTINEL_ADDRS=$sentinel_addrs REDIS_ADDR=$redis_addr"
}

preflight() {
  require_cmd kubectl
  validate_positive_integer "$TARGET_SECONDS" "MPP_REDIS_FAILOVER_TARGET_SECONDS"
  validate_positive_integer "$POLL_SECONDS" "MPP_REDIS_FAILOVER_POLL_SECONDS"
  validate_positive_integer "$REQUEST_TIMEOUT" "MPP_REDIS_FAILOVER_REQUEST_TIMEOUT"
  validate_nonprod_config

  require_can_i create pods/exec
  kubectl rollout status statefulset/redis-ha-primary -n "$APP_NS" "--timeout=${TARGET_SECONDS}s"
  kubectl rollout status statefulset/redis-ha-replica -n "$APP_NS" "--timeout=${TARGET_SECONDS}s"
  kubectl rollout status statefulset/redis-ha-sentinel -n "$APP_NS" "--timeout=${TARGET_SECONDS}s"
  kubectl rollout status deployment/backend -n "$APP_NS" "--timeout=${TARGET_SECONDS}s"
  kubectl rollout status deployment/publish-worker -n "$APP_NS" "--timeout=${TARGET_SECONDS}s"
  kubectl rollout status deployment/browser-worker -n "$APP_NS" "--timeout=${TARGET_SECONDS}s"
  backend_pods="$(select_backend_pods)"
  publish_worker_pods="$(select_publish_worker_pods)"
  browser_worker_pods="$(select_browser_worker_pods)"
  [[ -n "$backend_pods" ]] || fail "no running backend Pod found"
  [[ -n "$publish_worker_pods" ]] || fail "no running publish-worker Pod found"
  [[ -n "$browser_worker_pods" ]] || fail "no running browser-worker Pod found"
  set -- $backend_pods
  backend_probe_pod="$1"
  log "using backend Pod $backend_probe_pod for write probes"
  log "checking Redis readiness through backend=[$backend_pods] publish-worker=[$publish_worker_pods] browser-worker=[$browser_worker_pods]"

  workload_redis_mode_ok "$backend_pods" backend "backend"
  workload_redis_mode_ok "$publish_worker_pods" publish-worker "publish-worker"
  workload_redis_mode_ok "$browser_worker_pods" browser-worker "browser-worker"

  sentinel_exec 'redis-cli -p 26379 SENTINEL ckquorum "$REDIS_SENTINEL_MASTER_NAME"'
  app_clients_ready_probe || fail "application Redis client readiness failed before failover: $last_ready_detail"
  backend_write_probe "preflight" || fail "backend Redis write probe failed before failover: $last_write_detail"
}

validate_positive_integer() {
  local value="$1"
  local name="$2"
  [[ "$value" =~ ^[1-9][0-9]*$ ]] || fail "$name must be a positive integer; got $value"
}

diagnostics() {
  printf '\n== Redis HA failover diagnostics ==\n'
  printf 'namespace=%s master_name=%s drill_id=%s target_seconds=%s\n' "$APP_NS" "$MASTER_NAME" "$DRILL_ID" "$TARGET_SECONDS"
  printf 'backend_pods=%s\n' "$backend_pods"
  printf 'publish_worker_pods=%s\n' "$publish_worker_pods"
  printf 'browser_worker_pods=%s\n' "$browser_worker_pods"
  printf 'backend_probe_pod=%s\n' "$backend_probe_pod"
  printf 'last_master=%s\n' "$(one_line "$last_master")"
  printf 'last_ready_detail=%s\n' "$last_ready_detail"
  printf 'last_write_detail=%s\n' "$last_write_detail"
  printf '\n-- mpp-app-config Redis keys --\n'
  kubectl_allow_failure get configmap mpp-app-config -n "$APP_NS" \
    -o 'jsonpath={.data.APP_ENV}{" REDIS_ENDPOINT_MODE="}{.data.REDIS_ENDPOINT_MODE}{" REDIS_ADDR="}{.data.REDIS_ADDR}{" REDIS_SENTINEL_ADDRS="}{.data.REDIS_SENTINEL_ADDRS}{" REDIS_SENTINEL_MASTER_NAME="}{.data.REDIS_SENTINEL_MASTER_NAME}{"\n"}'
  printf '\n-- Sentinel master and quorum --\n'
  kubectl_allow_failure exec -n "$APP_NS" statefulset/redis-ha-sentinel -c sentinel -- \
    env REDIS_SENTINEL_MASTER_NAME="$MASTER_NAME" sh -ec '
      redis-cli -p 26379 SENTINEL get-master-addr-by-name "$REDIS_SENTINEL_MASTER_NAME"
      redis-cli -p 26379 SENTINEL ckquorum "$REDIS_SENTINEL_MASTER_NAME"
      redis-cli -p 26379 SENTINEL masters
    '
  printf '\n-- Redis HA Pods --\n'
  kubectl_allow_failure get pods -n "$APP_NS" \
    -l 'app.kubernetes.io/name=mpp,app.kubernetes.io/component in (redis-ha-primary,redis-ha-replica,redis-ha-sentinel)' \
    -o wide
  printf '\n-- App Pods --\n'
  kubectl_allow_failure get pods -n "$APP_NS" \
    -l 'app.kubernetes.io/name=mpp,app.kubernetes.io/component in (backend,publish-worker,browser-worker,collab-service)' \
    -o wide
  printf '\n-- Recent backend logs --\n'
  kubectl_allow_failure logs -n "$APP_NS" deployment/backend -c backend --tail=80 --since=10m
}

run_drill() {
  local before_master
  local start_epoch
  local deadline_epoch
  local attempt=0

  before_master="$(sentinel_master)"
  log "current Sentinel master: $(one_line "$before_master")"
  log "triggering Sentinel failover for $MASTER_NAME"
  trigger_sentinel_failover >/dev/null

  start_epoch="$(date +%s)"
  deadline_epoch=$((start_epoch + TARGET_SECONDS))

  while [[ "$(date +%s)" -le "$deadline_epoch" ]]; do
    attempt=$((attempt + 1))
    last_master="$(sentinel_master || true)"

    if [[ "$last_master" != "$before_master" ]] && app_clients_ready_probe && backend_write_probe "$attempt"; then
      local end_epoch
      local recovery_seconds
      end_epoch="$(date +%s)"
      recovery_seconds=$((end_epoch - start_epoch))

      log "failover complete"
      printf '\nRedis HA failover drill result\n'
      printf 'status=pass\n'
      printf 'drill_id=%s\n' "$DRILL_ID"
      printf 'namespace=%s\n' "$APP_NS"
      printf 'sentinel_master_name=%s\n' "$MASTER_NAME"
      printf 'master_before=%s\n' "$(one_line "$before_master")"
      printf 'master_after=%s\n' "$(one_line "$last_master")"
      printf 'observed_recovery_seconds=%s\n' "$recovery_seconds"
      printf 'target_recovery_seconds=%s\n' "$TARGET_SECONDS"
      printf 'backend_ready_probe=%s\n' "$last_ready_detail"
      printf 'backend_write_probe=%s\n' "$last_write_detail"
      return 0
    fi

    log "waiting for recovery: master=$(one_line "$last_master") ready=[$last_ready_detail] write=[$last_write_detail]"
    sleep "$POLL_SECONDS"
  done

  fail "Redis clients did not recover writes within ${TARGET_SECONDS}s"
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    preflight
    run_drill
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
