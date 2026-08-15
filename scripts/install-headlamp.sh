#!/usr/bin/env bash
set -euo pipefail

CLUSTER="${CLUSTER:-ai-infra-lab-v134}"
NAMESPACE="${HEADLAMP_NAMESPACE:-kube-system}"
ADMIN_SERVICE_ACCOUNT="${HEADLAMP_ADMIN_SERVICE_ACCOUNT:-headlamp-admin}"
HEADLAMP_IMAGE="${HEADLAMP_IMAGE:-m.daocloud.io/ghcr.io/headlamp-k8s/headlamp:latest}"
HEADLAMP_MANIFEST_URL="${HEADLAMP_MANIFEST_URL:-https://raw.githubusercontent.com/kubernetes-sigs/headlamp/main/kubernetes-headlamp.yaml}"

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${repo_root}/.tools/manifests/headlamp.yaml"

log() {
  printf '\n==> %s\n' "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

require_cmds() {
  local missing=0
  for cmd in curl docker kind kubectl sed; do
    if ! need_cmd "$cmd"; then
      echo "missing required command: $cmd" >&2
      missing=1
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    exit 1
  fi
}

load_image_into_kind() {
  if ! kind get clusters | grep -Fxq "$CLUSTER"; then
    log "Skipping kind image preload; cluster ${CLUSTER} was not found"
    return
  fi

  log "Pulling Headlamp image on the host"
  docker pull "$HEADLAMP_IMAGE"

  log "Loading Headlamp image into kind cluster ${CLUSTER}"
  if kind load docker-image "$HEADLAMP_IMAGE" --name "$CLUSTER"; then
    return
  fi

  log "Falling back to direct containerd import for each kind node"
  local archive
  archive="$(mktemp -t ai-infra-headlamp.XXXXXX.tar)"
  docker save "$HEADLAMP_IMAGE" -o "$archive"

  local node
  while IFS= read -r node; do
    docker exec --privileged -i "$node" \
      ctr --namespace=k8s.io images import --digests --snapshotter=overlayfs - <"$archive"
  done < <(docker ps \
    --filter "label=io.x-k8s.kind.cluster=${CLUSTER}" \
    --format '{{.Names}}')
  rm -f "$archive"
}

apply_headlamp() {
  log "Applying Headlamp manifest"
  mkdir -p "$(dirname "$manifest")"
  curl -fsSL "$HEADLAMP_MANIFEST_URL" -o "$manifest"
  sed -i "s#ghcr.io/headlamp-k8s/headlamp:latest#${HEADLAMP_IMAGE}#" "$manifest"

  kubectl apply -f "$manifest"
  kubectl -n "$NAMESPACE" patch deployment headlamp --type=strategic \
    -p '{"spec":{"template":{"spec":{"containers":[{"name":"headlamp","imagePullPolicy":"IfNotPresent"}]}}}}'
  kubectl -n "$NAMESPACE" rollout status deployment/headlamp --timeout=180s
}

create_admin_token_account() {
  log "Creating local Headlamp admin account"
  kubectl -n "$NAMESPACE" create serviceaccount "$ADMIN_SERVICE_ACCOUNT" \
    --dry-run=client -o yaml | kubectl apply -f -
  kubectl create clusterrolebinding "$ADMIN_SERVICE_ACCOUNT" \
    --clusterrole=cluster-admin \
    --serviceaccount="${NAMESPACE}:${ADMIN_SERVICE_ACCOUNT}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

print_usage() {
  cat <<EOF

Headlamp is ready.

Start the UI:

  kubectl -n ${NAMESPACE} port-forward --address 127.0.0.1 service/headlamp 4466:80

Open:

  http://127.0.0.1:4466

Create a login token:

  kubectl -n ${NAMESPACE} create token ${ADMIN_SERVICE_ACCOUNT} --duration=24h

EOF
}

main() {
  require_cmds
  load_image_into_kind
  apply_headlamp
  create_admin_token_account
  print_usage
}

main "$@"
