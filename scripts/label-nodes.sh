#!/usr/bin/env bash
set -euo pipefail

mapfile -t nodes < <(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name)
if (( ${#nodes[@]} < 3 )); then
  echo "need at least three worker nodes; create the cluster with kind.yaml" >&2
  exit 1
fi

kubectl label --overwrite "${nodes[0]}" infra.example.io/gpu-fabric=nvlink infra.example.io/rack=rack-a
kubectl label --overwrite "${nodes[1]}" infra.example.io/gpu-fabric=pcie infra.example.io/rack=rack-a
kubectl label --overwrite "${nodes[2]}" infra.example.io/gpu-fabric=pcie infra.example.io/rack=rack-b

for node in "${nodes[@]:0:3}"; do
  kubectl patch "$node" --subresource=status --type=merge \
    -p '{"status":{"capacity":{"example.com/gpu":"4"},"allocatable":{"example.com/gpu":"4"}}}'
done

kubectl get nodes -L infra.example.io/gpu-fabric,infra.example.io/rack
