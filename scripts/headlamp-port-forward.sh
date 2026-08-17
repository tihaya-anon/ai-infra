#!/usr/bin/env bash
set -euo pipefail

namespace="${HEADLAMP_NAMESPACE:-kube-system}"
port="${HEADLAMP_PORT:-4466}"
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." >/dev/null && pwd)"
pid_file="${HEADLAMP_PORT_FORWARD_PID:-${repo_root}/out/headlamp-port-forward.pid}"
log_file="${HEADLAMP_PORT_FORWARD_LOG:-${repo_root}/out/headlamp-port-forward.log}"

is_running() {
  local pid="$1"
  [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null
}

start() {
  local pid

  mkdir -p "$(dirname -- "$pid_file")" "$(dirname -- "$log_file")"
  if [[ -f "$pid_file" ]]; then
    pid="$(<"$pid_file")"
    if is_running "$pid"; then
      echo "Headlamp port-forward is already running (PID $pid)"
      echo "Headlamp URL: http://127.0.0.1:${port}"
      return
    fi
    rm -f "$pid_file"
  fi

  nohup kubectl -n "$namespace" port-forward --address 127.0.0.1 \
    service/headlamp "${port}:80" >"$log_file" 2>&1 &
  pid=$!
  echo "$pid" >"$pid_file"

  sleep 1
  if ! is_running "$pid"; then
    cat "$log_file"
    rm -f "$pid_file"
    return 1
  fi

  echo "Headlamp port-forward started (PID $pid)"
  echo "Headlamp URL: http://127.0.0.1:${port}"
  echo "Log: $log_file"
}

stop() {
  local pid

  if [[ ! -f "$pid_file" ]]; then
    echo "Headlamp port-forward is not running"
    return
  fi

  pid="$(<"$pid_file")"
  if [[ ! "$pid" =~ ^[0-9]+$ ]]; then
    echo "Invalid PID file: $pid_file" >&2
    return 1
  fi

  if is_running "$pid"; then
    kill "$pid"
    echo "Headlamp port-forward stopped (PID $pid)"
  else
    echo "Headlamp port-forward is not running (stale PID $pid)"
  fi
  rm -f "$pid_file"
}

case "${1:-}" in
  start)
    start
    ;;
  stop)
    stop
    ;;
  *)
    echo "Usage: $0 {start|stop}" >&2
    exit 2
    ;;
esac
