#!/usr/bin/env bash
set -euo pipefail

mapfile -t nodes < <(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name)
if (( ${#nodes[@]} < 3 )); then
  echo "need at least three worker nodes; create the cluster with kind.yaml" >&2
  exit 1
fi

kubectl label --overwrite "${nodes[0]}" infra.example.io/gpu-node=true infra.example.io/gpu-fabric=nvlink infra.example.io/rack=rack-a
kubectl label --overwrite "${nodes[1]}" infra.example.io/gpu-node=true infra.example.io/gpu-fabric=pcie infra.example.io/rack=rack-a
kubectl label --overwrite "${nodes[2]}" infra.example.io/gpu-node=true infra.example.io/gpu-fabric=pcie infra.example.io/rack=rack-b

kubectl -n ai-infra-system rollout status \
  daemonset/simulated-gpu-device-plugin --timeout=120s

for attempt in {1..24}; do
  ready=true
  for node in "${nodes[@]:0:3}"; do
    value="$(kubectl get "$node" -o jsonpath='{.status.allocatable.example\.com/gpu}')"
    if [[ "$value" != "4" ]]; then
      ready=false
      break
    fi
  done
  if [[ "$ready" == "true" ]]; then
    break
  fi
  if [[ "$attempt" == "24" ]]; then
    echo "simulated GPU device plugin did not register four devices per worker" >&2
    exit 1
  fi
  sleep 5
done

kubectl get nodes -L infra.example.io/gpu-node,infra.example.io/gpu-fabric,infra.example.io/rack
