#!/bin/sh
set -eu

max_length=100
status=0

for file in "$@"; do
    if awk 'NR <= 10 && /^\/\/ Code generated .* DO NOT EDIT\.$/ { found = 1 } END { exit !found }' "$file"; then
        continue
    fi

    if ! awk -v max="$max_length" '
        /^[[:space:]]*\/\/ \+kubebuilder:/ {
            next
        }
        length($0) > max {
            printf "%s:%d: line is %d characters; limit is %d\n", FILENAME, FNR, length($0), max
            failed = 1
        }
        END { exit failed }
    ' "$file"; then
        status=1
    fi
done

exit "$status"
