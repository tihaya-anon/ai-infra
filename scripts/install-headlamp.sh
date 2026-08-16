#!/usr/bin/env bash
set -euo pipefail

CLUSTER="${CLUSTER:-ai-infra-lab-v134}"
NAMESPACE="${HEADLAMP_NAMESPACE:-kube-system}"
ADMIN_SERVICE_ACCOUNT="${HEADLAMP_ADMIN_SERVICE_ACCOUNT:-headlamp-admin}"
HEADLAMP_IMAGE="${HEADLAMP_IMAGE:-m.daocloud.io/ghcr.io/headlamp-k8s/headlamp:latest}"
HEADLAMP_MANIFEST_URL="${HEADLAMP_MANIFEST_URL:-https://raw.githubusercontent.com/kubernetes-sigs/headlamp/main/kubernetes-headlamp.yaml}"

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
manifest_dir="${repo_root}/.tools/manifests/headlamp"
manifest="${manifest_dir}/base.yaml"

log() {
  printf '\n==> %s\n' "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

require_cmds() {
  local missing=0
  for cmd in curl kubectl sed; do
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
  log "Loading Headlamp image into kind cluster ${CLUSTER}"
  CLUSTER="$CLUSTER" KIND_IMAGE_MIRROR_PREFIX= "${repo_root}/scripts/load-kind-images.sh" "$HEADLAMP_IMAGE"
}

apply_headlamp() {
  log "Applying Headlamp manifest"
  mkdir -p "$manifest_dir"
  curl -fsSL "$HEADLAMP_MANIFEST_URL" -o "$manifest"
  sed -i "s#ghcr.io/headlamp-k8s/headlamp:latest#${HEADLAMP_IMAGE}#" "$manifest"

  cat >"${manifest_dir}/controller-manager-patch.yaml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: headlamp
  namespace: ${NAMESPACE}
spec:
  template:
    spec:
      containers:
        - name: headlamp
          imagePullPolicy: IfNotPresent
EOF

  cat >"${manifest_dir}/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - base.yaml
patches:
  - path: controller-manager-patch.yaml
EOF

  kubectl apply -k "$manifest_dir"
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
