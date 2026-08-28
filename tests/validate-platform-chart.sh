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

# metrics-server backs the admin panel's usage figures and nothing else, so it is
# conditional. Assert both halves: it renders by default, and a cluster that
# already has one (k3s ships it) can switch it off without disabling anything else
# — two aggregated API servers cannot both back metrics.k8s.io.
grep -q 'name: metrics-server' "$output_file"
grep -q -- '- --kubelet-insecure-tls' "$output_file"

without_metrics=$(helm template home-lab-platform "${repo_root}/charts/platform" \
  --namespace platform-system \
  --kube-version "$kube_version" \
  --api-versions policy/v1/PodDisruptionBudget \
  --values "${repo_root}/charts/platform/values.local.yaml.example" \
  --set metrics-server.enabled=false)
if grep -q 'name: metrics-server' <<<"$without_metrics"; then
  printf 'Disabling metrics-server must leave nothing of it behind\n' >&2
  exit 1
fi
grep -q '^kind: Gateway$' <<<"$without_metrics"

# --- the cluster's storage keeps what it is given -----------------------------
#
# One class, and deleting a claim must not destroy its data. Two settings say so
# together, and the pairing is the whole assertion.
#
# reclaimPolicy MUST be Delete, which is the opposite of how it reads. It selects
# which handler runs, and only Delete hands the volume to this provisioner —
# Retain leaves the PV Released and never calls it, so archiveOnDelete would be
# read by nobody and nothing would ever be renamed. So `Retain` here is the
# regression, not the safe choice, and it is asserted against by name because it
# is what somebody reaching for safety would reach for.
storage_class=$(awk '/^kind: StorageClass$/,/^---$/' "$output_file")
grep -q 'reclaimPolicy: Delete' <<<"$storage_class"
grep -q 'archiveOnDelete: "true"' <<<"$storage_class"
if grep -q 'archiveOnDelete: "false"' <<<"$storage_class"; then
  printf 'archiveOnDelete must be true; deleting a claim would otherwise destroy its data\n' >&2
  exit 1
fi
if grep -q 'reclaimPolicy: Retain' <<<"$storage_class"; then
  printf 'reclaimPolicy must be Delete, or the provisioner never runs and nothing is archived\n' >&2
  exit 1
fi
# onDelete overrides archiveOnDelete whenever it is set, so naming both is two
# settings answering one question with only one of them winning.
if grep -q 'onDelete:' <<<"$storage_class"; then
  printf 'onDelete overrides archiveOnDelete; set one of them, not both\n' >&2
  exit 1
fi

# --- Redis is off until something needs it ------------------------------------
if grep -q 'app.kubernetes.io/name: redis' "$output_file"; then
  printf 'Redis must be disabled by default; it is a standing credential with no caller yet\n' >&2
  exit 1
fi

redis=$(helm template home-lab-platform "${repo_root}/charts/platform" \
  --namespace platform-system \
  --kube-version "$kube_version" \
  --api-versions policy/v1/PodDisruptionBudget \
  --values "${repo_root}/charts/platform/values.local.yaml.example" \
  --set platform.redis.enabled=true \
  --show-only templates/redis.yaml)

# One replica, and not for want of raising it: a lock has to have one authority,
# so a second Redis is a second answer to a question that must have exactly one.
# Recreate is the second half — the default RollingUpdate runs two pods behind one
# Service for a few seconds, which is the same failure for a shorter time.
redis_deploy=$(awk '/^kind: Deployment$/,/^---$/' <<<"$redis")
grep -q '^  replicas: 1$' <<<"$redis_deploy"
grep -q 'type: Recreate' <<<"$redis_deploy"

# Nothing durable lives here, and the flags are what say so. BOTH are asserted
# because they disable different mechanisms: --save "" stops RDB snapshots and
# --appendonly no stops the append-only log, and either one alone leaves the pod
# writing state it is not supposed to keep.
grep -q -- '--save ""' <<<"$redis_deploy"
grep -q -- '--appendonly no' <<<"$redis_deploy"

# noeviction, deliberately. This holds locks, and an evicted lock is not a lock —
# reaching the memory ceiling has to fail a write loudly instead.
grep -q -- '--maxmemory-policy noeviction' <<<"$redis_deploy"

# The password comes from a Secret rather than from values; this repository is
# public. Asserted on the REDIS_PASSWORD entry ITSELF rather than as two
# independent greps: separate checks for the name and for a secretKeyRef both pass
# on a manifest where the secret reference belongs to some other variable and the
# password is inline, which is exactly the state worth failing on.
redis_password_env=$(grep -A3 'name: REDIS_PASSWORD' <<<"$redis_deploy")
grep -q 'valueFrom' <<<"$redis_password_env"
grep -q 'secretKeyRef' <<<"$redis_password_env"

# ...and authentication is not the only control. The NetworkPolicy is what says a
# leaked credential can only be spent from a namespace somebody labelled.
#
# Checked where the label actually has to be — under the ingress rule's
# namespaceSelector — rather than anywhere in the document. The loose form passes
# on a policy that merely mentions the label in a comment or carries it as its own
# metadata, neither of which admits anybody.
grep -q 'kind: NetworkPolicy' <<<"$redis"
grep -A2 'namespaceSelector:' <<<"$redis" | grep -q 'home-lab.example/redis-access: "true"'

printf 'Platform chart validation passed.\n'
