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
# One class, and it retains. This was Delete with archiveOnDelete false, and the
# first time a claim holding real data was removed that turned out to be the wrong
# default. Both halves are asserted because they do different jobs: reclaimPolicy
# leaves the PV Released so the data has something to be reattached through, and
# archiveOnDelete leaves the directory on the export instead of emptying it.
# Retain with archiveOnDelete false is a PV pointing at nothing, which reads as
# safe and is not.
storage_class=$(awk '/^kind: StorageClass$/,/^---$/' "$output_file")
grep -q 'reclaimPolicy: Retain' <<<"$storage_class"
grep -q 'archiveOnDelete: "true"' <<<"$storage_class"
if grep -q 'reclaimPolicy: Delete' <<<"$storage_class"; then
  printf 'The default StorageClass must retain; deleting a claim must not destroy its data\n' >&2
  exit 1
fi
# onDelete overrides archiveOnDelete when set, so naming both is two settings
# answering one question with only one of them winning.
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

# Nothing durable lives here, and the flags are what say so. Persistence off both
# ways: --save "" stops snapshots, --appendonly no stops the log.
grep -q -- '--appendonly no' <<<"$redis_deploy"

# noeviction, deliberately. This holds locks, and an evicted lock is not a lock —
# reaching the memory ceiling has to fail a write loudly instead.
grep -q -- '--maxmemory-policy noeviction' <<<"$redis_deploy"

# The password comes from a Secret rather than from values; this repository is
# public. Asserted as the wiring rather than as "no literal", so a value moved
# back inline fails here.
grep -q 'secretKeyRef' <<<"$redis_deploy"
grep -q 'name: REDIS_PASSWORD' <<<"$redis_deploy"

# ...and authentication is not the only control. The NetworkPolicy is what says a
# leaked credential can only be spent from a namespace somebody labelled.
grep -q 'kind: NetworkPolicy' <<<"$redis"
grep -q 'home-lab.example/redis-access' <<<"$redis"

printf 'Platform chart validation passed.\n'
