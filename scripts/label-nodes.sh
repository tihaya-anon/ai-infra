#!/usr/bin/env bash
set -euo pipefail

mapfile -t nodes < <(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name)
if (( ${#nodes[@]} < 3 )); then
  echo "need at least three worker nodes; create the cluster with kind.yaml" >&2
  exit 1
fi

kubectl label --overwrite "${nodes[0]}" infra.example.io/gpu-capacity=4 infra.example.io/rack=rack-a
kubectl label --overwrite "${nodes[1]}" infra.example.io/gpu-capacity=4 infra.example.io/rack=rack-a
kubectl label --overwrite "${nodes[2]}" infra.example.io/gpu-capacity=4 infra.example.io/rack=rack-b

kubectl get nodes -L infra.example.io/gpu-capacity,infra.example.io/rack
