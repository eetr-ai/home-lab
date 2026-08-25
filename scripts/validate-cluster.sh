#!/usr/bin/env bash
set -euo pipefail

: "${KUBECONFIG:?Set KUBECONFIG to the ignored Ansible artifact path}"

for command_name in cilium grep kubectl mktemp openssl; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
done

expected_version='v1.36.4'
expected_nodes=3

node_count=$(kubectl get nodes -o name | wc -l | tr -d ' ')
if [[ $node_count != "$expected_nodes" ]]; then
  printf 'Expected %s nodes, found %s\n' "$expected_nodes" "$node_count" >&2
  exit 1
fi

if ! kubectl get nodes \
  -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.nodeInfo.kubeletVersion}{"\n"}{end}' |
  awk -v expected="$expected_version" '$2 != expected {bad=1} END {exit bad}'; then
  printf 'At least one node is not running %s\n' "$expected_version" >&2
  exit 1
fi

kubectl wait --for=condition=Ready nodes --all --timeout=5m
cilium status --wait
cilium connectivity test

control_plane=$(kubectl get nodes -l node-role.kubernetes.io/control-plane \
  -o jsonpath='{.items[0].metadata.name}')
if [[ -z $control_plane ]]; then
  printf 'Could not identify the control-plane node\n' >&2
  exit 1
fi

secret_name='encryption-at-rest-check'
secret_marker=$(openssl rand -hex 16)
etcd_record=$(mktemp -t home-lab-etcd-record.XXXXXX)

cleanup() {
  kubectl delete secret "$secret_name" --ignore-not-found >/dev/null 2>&1 || true
  rm -f "$etcd_record"
}
trap cleanup EXIT

kubectl create secret generic "$secret_name" --from-literal="value=$secret_marker" >/dev/null

kubectl -n kube-system exec "etcd-$control_plane" -- \
  etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  get "/registry/secrets/default/$secret_name" \
  --print-value-only >"$etcd_record"

if ! grep -aFq 'k8s:enc:secretbox:v1:' "$etcd_record"; then
  printf 'The raw etcd record does not use the secretbox envelope\n' >&2
  exit 1
fi

if grep -aFq "$secret_marker" "$etcd_record"; then
  printf 'The raw etcd record contains the plaintext test value\n' >&2
  exit 1
fi

printf 'Cluster, Cilium, connectivity, and secretbox validation passed.\n'
