#!/usr/bin/env bash
set -euo pipefail

attempts="${KUBECTL_APPLY_ATTEMPTS:-12}"
delay="${KUBECTL_APPLY_RETRY_DELAY:-5}"

if (( $# == 0 )); then
  echo "usage: $0 kubectl-apply-arguments..." >&2
  exit 2
fi

for ((attempt = 1; attempt <= attempts; attempt++)); do
  if kubectl apply "$@"; then
    exit 0
  fi
  if (( attempt == attempts )); then
    echo "kubectl apply failed after ${attempts} attempts" >&2
    exit 1
  fi
  echo "kubectl apply attempt ${attempt}/${attempts} failed; retrying in ${delay}s" >&2
  sleep "$delay"
done
