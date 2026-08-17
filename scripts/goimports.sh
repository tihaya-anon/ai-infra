#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tool_dir="$repo_root/.tools/bin"
goimports="$tool_dir/goimports"

if [ ! -x "$goimports" ]; then
    mkdir -p "$tool_dir"
    env GOTOOLCHAIN=go1.25.0 GOBIN="$tool_dir" \
        go install golang.org/x/tools/cmd/goimports@v0.42.0
fi

if [ "${1:-}" = "--install" ]; then
    exit 0
fi

exec "$goimports" "$@"
