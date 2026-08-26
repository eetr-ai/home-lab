#!/usr/bin/env bash

# Render the platform chart and assert the properties the cluster depends on.
#
# These checks used to live inline in the repository-checks workflow. They are a
# script so the same assertions run locally through `task tests:chart`, and so the
# repository-wide shell lint covers them like every other script here.
#
# The rendered document is left behind on purpose: kubeconform reads it as a
# separate step in CI.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_file="${1:-${repo_root}/rendered-platform.yaml}"
kube_version="${KUBE_VERSION:-1.36.0}"

helm template home-lab-platform "${repo_root}/charts/platform" \
  --namespace platform-system \
  --kube-version "$kube_version" \
  --api-versions policy/v1/PodDisruptionBudget \
  --values "${repo_root}/charts/platform/values.local.yaml.example" \
  >"$output_file"

# Gateway API routing is the whole point of the chart, so its two kinds must be
# present and the whoami probe must render with the arguments it is given.
grep -q '^kind: Gateway$' "$output_file"
grep -q '^kind: HTTPRoute$' "$output_file"
grep -q -- '- --port=8080' "$output_file"

# Every workload runs unprivileged as the same non-root user.
grep -q 'runAsUser: 65532' "$output_file"

# Traefik must stay ClusterIP. The cluster has no LoadBalancer and public traffic
# arrives through cloudflared, so a Service type change would silently ask for an
# address nothing can provide.
awk '
  $0 == "# Source: home-lab-platform/charts/traefik/templates/service.yaml" {
    in_traefik_service = 1
  }
  in_traefik_service && $1 == "type:" {
    found = 1
    if ($2 != "ClusterIP") exit 1
    exit 0
  }
  END { if (!found) exit 1 }
' "$output_file"

# Ingress and Gateway API are two ways to say the same thing, and mixing them
# splits routing across two mechanisms that have to be reasoned about together.
if grep -q '^kind: Ingress$' "$output_file"; then
  printf 'Platform chart must use Gateway API, not Ingress\n' >&2
  exit 1
fi

printf 'Platform chart validation passed.\n'
