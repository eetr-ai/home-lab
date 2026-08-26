#!/usr/bin/env bash

# Install or upgrade the admin panel release.
#
# Mirrors install-platform.sh: refuse a values file that still holds placeholders,
# make sure the namespace can attach a route, and prove every Secret the chart
# references already exists before Helm creates pods that would fail on it.

set -euo pipefail

usage() {
  printf 'Usage: %s --values /absolute/or/relative/values.local.yaml\n' "$0" >&2
}

values_file=''
while (($#)); do
  case "$1" in
    --values)
      [[ $# -ge 2 ]] || {
        usage
        exit 2
      }
      values_file=$2
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

[[ -n $values_file && -f $values_file ]] || {
  usage
  exit 2
}
: "${KUBECONFIG:?Set KUBECONFIG to the ignored administrator kubeconfig}"

for command_name in awk grep helm kubectl; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  }
done

if grep -Eq 'example\.(com|invalid)' "$values_file"; then
  printf 'Replace every example registry and hostname in %s\n' "$values_file" >&2
  exit 1
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
chart_dir="$repo_root/charts/admin"
namespace='admin'
release='home-lab-admin'

kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -

# The platform Gateway only admits routes from namespaces carrying this label, so
# applying it here is what lets the panel be reachable at all.
kubectl label namespace "$namespace" \
  home-lab.example/gateway-access=true \
  --overwrite

rendered_chart=$(helm template "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file")

# Read the pull secrets out of what the chart actually renders rather than out of
# the values file. The chart is what the cluster will act on, and a name that
# reaches a pod without a matching Secret is an ImagePullBackOff several minutes
# later with nothing pointing at the cause.
#
# Read with a while loop rather than mapfile: this script runs from the operator's
# laptop, and macOS still ships bash 3.2, which has no mapfile.
pull_secrets=$(printf '%s\n' "$rendered_chart" | awk '
  $1 == "imagePullSecrets:" { in_list = 1; next }
  in_list && $1 == "-" && $2 == "name:" { print $3; next }
  in_list { in_list = 0 }
' | sort -u)

while read -r secret_name; do
  [[ -n $secret_name ]] || continue
  secret_type=$(kubectl get secret "$secret_name" --namespace "$namespace" \
    -o 'go-template={{ .type }}' 2>/dev/null || true)
  if [[ $secret_type != 'kubernetes.io/dockerconfigjson' ]]; then
    printf 'Secret %s/%s must exist and be of type kubernetes.io/dockerconfigjson.\n' \
      "$namespace" "$secret_name" >&2
    printf 'See charts/admin/README.md for the command that creates it.\n' >&2
    exit 1
  fi
done <<<"$pull_secrets"

# Every Secret the rendered chart reads a value out of. Same reasoning as the pull
# secret: read from what the cluster will act on, and fail here rather than as a
# pod that starts and immediately exits with a missing environment variable.
value_secrets=$(printf '%s\n' "$rendered_chart" | awk '
  $1 == "secretKeyRef:" { in_ref = 1; next }
  in_ref && $1 == "name:" { name = $2; next }
  in_ref && $1 == "key:"  { print name ":" $2; in_ref = 0 }
' | sort -u)

while read -r secret_spec; do
  [[ -n $secret_spec ]] || continue
  secret_name=${secret_spec%%:*}
  secret_key=${secret_spec#*:}
  if [[ -z $(kubectl get secret "$secret_name" --namespace "$namespace" \
    -o "go-template={{ index .data \"$secret_key\" }}" 2>/dev/null) ]]; then
    printf 'Secret %s/%s must exist with key %s\n' \
      "$namespace" "$secret_name" "$secret_key" >&2
    printf 'See charts/admin/README.md for the commands that create it.\n' >&2
    exit 1
  fi
done <<<"$value_secrets"

helm upgrade --install "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file" \
  --rollback-on-failure \
  --wait \
  --timeout 5m

printf 'Admin release installed.\n'
