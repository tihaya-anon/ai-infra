#!/usr/bin/env bash
set -euo pipefail

namespace="${HEADLAMP_NAMESPACE:-kube-system}"
service_account="${HEADLAMP_ADMIN_SERVICE_ACCOUNT:-headlamp-admin}"
duration="${HEADLAMP_TOKEN_DURATION:-24h}"

kubectl -n "$namespace" create token "$service_account" --duration="$duration"
