#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tool_dir="$repo_root/.tools/bin"
controller_gen="$tool_dir/controller-gen"

if [ ! -x "$controller_gen" ]; then
    mkdir -p "$tool_dir"
    env GOTOOLCHAIN=go1.25.0 GOBIN="$tool_dir" \
        go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0
fi

if [ "${1:-}" = "--install" ]; then
    exit 0
fi

exec "$controller_gen" "$@"
