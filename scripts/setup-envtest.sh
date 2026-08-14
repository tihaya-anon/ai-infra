#!/usr/bin/env bash
set -euo pipefail

version="${1:-1.34.0}"
if [[ "$version" != "1.34.0" ]]; then
  echo "unsupported envtest version: $version" >&2
  exit 1
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)
    platform="linux-amd64"
    checksum="445b4dd543963744925fffd5225413d852821442b244db710057abc3a98cf5a1e139a43b4b97949010360a19a8ffab4971db97daba60d72ddf1cc260cbfe3844"
    ;;
  Linux-aarch64 | Linux-arm64)
    platform="linux-arm64"
    checksum="06f3b72b133595f463849d8cde7c36307e2e51e1709bded1659694c1bd39491d3133d534d33a7c38df1136c314d1e1ad483684c519c0694f505b703ed2e44ef9"
    ;;
  Darwin-x86_64)
    platform="darwin-amd64"
    checksum="e92e41a183e8504d3763bd5486418167ce4f6204556d3a53c7abc2e538da639db940f8a1764030d8bd5c5658cbc3eca35e1c1749290b74fc1653da6e517fc995"
    ;;
  Darwin-arm64)
    platform="darwin-arm64"
    checksum="b841e5ffa351b2ea1748093b612bc63e23a36362c90304cec498f1ee29c2858b63c146012a562f39d92031dba68a819a381d71c348b567c9de986c069edfce04"
    ;;
  *)
    echo "unsupported envtest platform: $(uname -s)-$(uname -m)" >&2
    exit 1
    ;;
esac

root=".tools/envtest/${version}-${platform}"
assets="${root}/controller-tools/envtest"
if [[ -x "${assets}/kube-apiserver" && -x "${assets}/etcd" ]]; then
  printf '%s\n' "$(cd "$assets" && pwd)"
  exit 0
fi

mkdir -p "$root"
archive="${root}/envtest.tar.gz"
url="https://github.com/kubernetes-sigs/controller-tools/releases/download/envtest-v${version}/envtest-v${version}-${platform}.tar.gz"

checksum_file() {
  if command -v sha512sum >/dev/null 2>&1; then
    sha512sum "$1" | awk '{print $1}'
  else
    shasum -a 512 "$1" | awk '{print $1}'
  fi
}

actual=""
if [[ -f "$archive" ]]; then
  actual="$(checksum_file "$archive")"
fi
if [[ "$actual" != "$checksum" ]]; then
  curl -fL -C - "$url" -o "$archive"
  actual="$(checksum_file "$archive")"
fi
if [[ "$actual" != "$checksum" ]]; then
  echo "envtest checksum mismatch: got $actual" >&2
  exit 1
fi

tar -xzf "$archive" -C "$root"
printf '%s\n' "$(cd "$assets" && pwd)"
