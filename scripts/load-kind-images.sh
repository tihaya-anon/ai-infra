#!/usr/bin/env bash
set -euo pipefail

CLUSTER="${CLUSTER:-ai-infra-lab-v134}"
KIND_IMAGE_PLATFORM="${KIND_IMAGE_PLATFORM:-}"
KIND_IMAGE_MIRROR_PREFIX="${KIND_IMAGE_MIRROR_PREFIX:-}"

tmp_files=()

cleanup() {
  local file
  for file in "${tmp_files[@]}"; do
    rm -f "$file"
  done
}

trap cleanup EXIT

log() {
  printf '\n==> %s\n' "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

require_cmds() {
  local missing=0
  for cmd in docker kind; do
    if ! need_cmd "$cmd"; then
      echo "missing required command: $cmd" >&2
      missing=1
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    exit 1
  fi
}

detect_platform() {
  case "$(uname -m)" in
    x86_64 | amd64)
      printf 'linux/amd64\n'
      ;;
    aarch64 | arm64)
      printf 'linux/arm64\n'
      ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

image_source() {
  local image="$1"
  if [[ -n "$KIND_IMAGE_MIRROR_PREFIX" ]]; then
    printf '%s/%s\n' "${KIND_IMAGE_MIRROR_PREFIX%/}" "$image"
  else
    printf '%s\n' "$image"
  fi
}

kind_nodes() {
  docker ps \
    --filter "label=io.x-k8s.kind.cluster=${CLUSTER}" \
    --format '{{.Names}}'
}

pull_image() {
  local image="$1"
  local source
  source="$(image_source "$image")"

  log "Pulling ${source} for ${KIND_IMAGE_PLATFORM}"
  docker pull --platform "$KIND_IMAGE_PLATFORM" "$source"

  if [[ "$source" != "$image" ]]; then
    docker tag "$source" "$image"
  fi
}

load_with_ctr() {
  local image="$1"
  local archive node
  archive="$(mktemp -t ai-infra-kind-image.XXXXXX.tar)"
  tmp_files+=("$archive")

  log "Exporting ${image} for ${KIND_IMAGE_PLATFORM}"
  docker save --platform "$KIND_IMAGE_PLATFORM" -o "$archive" "$image"

  while IFS= read -r node; do
    [[ -n "$node" ]] || continue
    log "Importing ${image} into ${node}"
    docker exec --privileged -i "$node" \
      ctr --namespace=k8s.io images import --digests --snapshotter=overlayfs - <"$archive"
  done < <(kind_nodes)
}

load_image() {
  local image="$1"
  pull_image "$image"

  log "Loading ${image} into kind cluster ${CLUSTER}"
  load_with_ctr "$image"
}

main() {
  if [[ "$#" -eq 0 ]]; then
    echo "usage: CLUSTER=name $0 image[:tag] [image[:tag] ...]" >&2
    exit 1
  fi

  require_cmds
  KIND_IMAGE_PLATFORM="${KIND_IMAGE_PLATFORM:-$(detect_platform)}"

  if ! kind get clusters | grep -Fxq "$CLUSTER"; then
    echo "kind cluster not found: ${CLUSTER}" >&2
    exit 1
  fi

  local image
  for image in "$@"; do
    load_image "$image"
  done
}

main "$@"
