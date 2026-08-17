#!/usr/bin/env bash
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.25.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-1.34.8}"
KIND_VERSION="${KIND_VERSION:-v0.32.0}"
GOPROXY_VALUE="${GOPROXY_VALUE:-https://goproxy.cn,direct}"

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

log() {
  printf '\n==> %s\n' "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

sudo_cmd() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

require_linux() {
  if [[ "$(uname -s)" != "Linux" ]]; then
    echo "this installer currently supports Linux only" >&2
    exit 1
  fi
  if [[ ! -r /etc/os-release ]]; then
    echo "cannot detect Linux distribution: /etc/os-release is missing" >&2
    exit 1
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64)
      printf 'amd64\n'
      ;;
    aarch64 | arm64)
      printf 'arm64\n'
      ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

detect_debian_family() {
  # shellcheck disable=SC1091
  . /etc/os-release
  case "${ID:-}" in
    ubuntu | debian)
      docker_repo_id="$ID"
      ;;
    *)
      case "${ID_LIKE:-}" in
        *ubuntu*)
          docker_repo_id="ubuntu"
          ;;
        *debian*)
          docker_repo_id="debian"
          ;;
        *)
          echo "unsupported distribution: ${PRETTY_NAME:-unknown}" >&2
          echo "this installer expects Ubuntu/Debian or a close derivative" >&2
          exit 1
          ;;
      esac
      ;;
  esac

  docker_repo_codename="${UBUNTU_CODENAME:-${DEBIAN_CODENAME:-${VERSION_CODENAME:-}}}"
  if [[ -z "$docker_repo_codename" ]]; then
    echo "cannot detect distribution codename from /etc/os-release" >&2
    exit 1
  fi

  printf '%s:%s\n' "$docker_repo_id" "$docker_repo_codename"
}

install_apt_basics() {
  log "Installing base packages"
  sudo_cmd apt-get update
  sudo_cmd apt-get install -y \
    bash \
    ca-certificates \
    coreutils \
    curl \
    findutils \
    git \
    gnupg \
    make \
    pre-commit \
    python3 \
    tar
}

install_go() {
  local arch="$1"
  local wanted="go${GO_VERSION}"
  local current=""

  if need_cmd go; then
    current="$(go version | awk '{print $3}')"
  fi
  if [[ "$current" == "$wanted" ]]; then
    log "Go ${GO_VERSION} already installed"
  else
    log "Installing Go ${GO_VERSION}"
    local tmpdir
    tmpdir="$(mktemp -d)"
    curl -fL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" \
      -o "${tmpdir}/go.tar.gz"
    sudo_cmd rm -rf /usr/local/go
    sudo_cmd tar -C /usr/local -xzf "${tmpdir}/go.tar.gz"
    rm -rf "$tmpdir"
  fi

  sudo_cmd tee /etc/profile.d/ai-infra-lab-go.sh >/dev/null <<'EOF'
export PATH="/usr/local/go/bin:${PATH}"
EOF
  export PATH="/usr/local/go/bin:${PATH}"
  go env -w "GOPROXY=${GOPROXY_VALUE}"
}

install_kubectl() {
  local arch="$1"
  local wanted="v${KUBECTL_VERSION}"
  local current=""

  if need_cmd kubectl; then
    current="$(kubectl version --client=true -o yaml 2>/dev/null \
      | awk '/gitVersion:/ {print $2; exit}')"
  fi
  if [[ "$current" == "$wanted" ]]; then
    log "kubectl ${KUBECTL_VERSION} already installed"
    return
  fi

  log "Installing kubectl ${KUBECTL_VERSION}"
  local tmpdir
  tmpdir="$(mktemp -d)"
  curl -fL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${arch}/kubectl" \
    -o "${tmpdir}/kubectl"
  curl -fL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${arch}/kubectl.sha256" \
    -o "${tmpdir}/kubectl.sha256"
  printf '%s  %s\n' "$(cat "${tmpdir}/kubectl.sha256")" "${tmpdir}/kubectl" \
    | sha256sum --check
  chmod +x "${tmpdir}/kubectl"
  sudo_cmd install -m 0755 "${tmpdir}/kubectl" /usr/local/bin/kubectl
  rm -rf "$tmpdir"
}

install_kind() {
  local arch="$1"
  local current=""

  if need_cmd kind; then
    current="$(kind version | awk '{print $2}')"
  fi
  if [[ "$current" == "$KIND_VERSION" ]]; then
    log "kind ${KIND_VERSION} already installed"
    return
  fi

  log "Installing kind ${KIND_VERSION}"
  local tmpdir
  tmpdir="$(mktemp -d)"
  curl -fL "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${arch}" \
    -o "${tmpdir}/kind"
  chmod +x "${tmpdir}/kind"
  sudo_cmd install -m 0755 "${tmpdir}/kind" /usr/local/bin/kind
  rm -rf "$tmpdir"
}

install_docker() {
  local repo_id="$1"
  local codename="$2"
  local arch="$3"

  if need_cmd docker; then
    log "Docker already installed"
  else
    log "Installing Docker Engine"
    for pkg in docker.io docker-doc docker-compose podman-docker containerd runc; do
      sudo_cmd apt-get remove -y "$pkg" >/dev/null 2>&1 || true
    done

    sudo_cmd install -m 0755 -d /etc/apt/keyrings
    sudo_cmd rm -f /tmp/ai-infra-lab-docker.gpg
    curl -fsSL "https://download.docker.com/linux/${repo_id}/gpg" \
      | sudo_cmd gpg --dearmor -o /tmp/ai-infra-lab-docker.gpg
    sudo_cmd install -m 0644 /tmp/ai-infra-lab-docker.gpg /etc/apt/keyrings/docker.gpg
    sudo_cmd rm -f /tmp/ai-infra-lab-docker.gpg

    printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/%s %s stable\n' \
      "$arch" "$repo_id" "$codename" \
      | sudo_cmd tee /etc/apt/sources.list.d/docker.list >/dev/null

    sudo_cmd apt-get update
    sudo_cmd apt-get install -y docker-ce docker-ce-cli containerd.io \
      docker-buildx-plugin docker-compose-plugin
  fi

  if need_cmd systemctl; then
    sudo_cmd systemctl enable --now docker
  fi

  local target_user="${SUDO_USER:-${USER:-}}"
  if [[ -n "$target_user" && "$target_user" != "root" ]]; then
    sudo_cmd groupadd -f docker
    sudo_cmd usermod -aG docker "$target_user"
  fi
}

install_project_tools() {
  log "Installing project-local Go tools"
  (cd "$repo_root" && env GOTOOLCHAIN="go${GO_VERSION}" GOPROXY="$GOPROXY_VALUE" make tools)
}

print_versions() {
  log "Installed versions"
  go version
  kubectl version --client=true
  kind version
  docker version --format 'Docker client {{.Client.Version}}'
  make --version | head -n 1
  pre-commit --version
}

main() {
  require_linux

  local arch repo_info repo_id codename
  arch="$(detect_arch)"
  repo_info="$(detect_debian_family)"
  repo_id="${repo_info%%:*}"
  codename="${repo_info##*:}"

  install_apt_basics
  install_go "$arch"
  install_kubectl "$arch"
  install_kind "$arch"
  install_docker "$repo_id" "$codename" "$arch"
  install_project_tools
  print_versions

  cat <<EOF

Docker group membership may require a new login shell before docker works
without sudo. After that, run:

  make verify
  make cluster CLUSTER=ai-infra-lab-v134

EOF
}

main "$@"
