#!/usr/bin/env bash

# Render the admin chart and assert the properties the cluster depends on.
#
# The rendered document is left behind on purpose: kubeconform reads it as a
# separate step in CI.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_file="${1:-${repo_root}/rendered-admin.yaml}"
kube_version="${KUBE_VERSION:-1.36.0}"
values_file="${repo_root}/charts/admin/values.local.yaml.example"

render() {
  helm template home-lab-admin "${repo_root}/charts/admin" \
    --namespace admin \
    --kube-version "$kube_version" \
    --values "$values_file" \
    "$@"
}

render >"$output_file"

grep -q '^kind: Deployment$' "$output_file"
grep -q '^kind: Service$' "$output_file"
grep -q '^kind: ServiceAccount$' "$output_file"

# The pod runs unprivileged as the same non-root user every workload here uses,
# and writes nothing to its filesystem.
grep -q 'runAsUser: 65532' "$output_file"
grep -q 'readOnlyRootFilesystem: true' "$output_file"

# The nodes hold the registry credential, so a per-namespace pull secret is an
# override rather than the norm. Assert both halves: nothing is emitted by default,
# and what is asked for is passed through.
if grep -q 'imagePullSecrets:' "$output_file"; then
  printf 'The chart must not require a per-namespace pull secret by default\n' >&2
  exit 1
fi
render --set admin.imagePullSecrets[0].name=some-secret | grep -q 'name: some-secret'

# The API refuses to start without an issuer, and the template makes that a render
# failure rather than a runtime one: `required` stops Helm before Kubernetes sees a
# Deployment. Assert the wiring exists, and that both an empty issuer and a scheme
# with no host are refused.
grep -q 'name: ADMIN_OIDC_ISSUER' "$output_file"

if render --set admin.api.oidc.issuer= >/dev/null 2>&1; then
  printf 'The chart rendered with no OIDC issuer\n' >&2
  exit 1
fi

if render --set admin.api.oidc.issuer=https:// >/dev/null 2>&1; then
  printf 'The chart rendered with a hostless OIDC issuer\n' >&2
  exit 1
fi

# A moving tag cannot say which build is running, and the chart's appVersion is
# what release-please keeps in step with the images.
if grep -qE 'image: .*:latest"?$' "$output_file"; then
  printf 'Admin chart must reference an explicit image tag, not latest\n' >&2
  exit 1
fi

if grep -q '^kind: Ingress$' "$output_file"; then
  printf 'Admin chart must use Gateway API, not Ingress\n' >&2
  exit 1
fi

# The API is not routable from outside by default. That default is the point:
# enabling it is a decision, taken after Cloudflare Access is in front of the
# hostname, and a chart that quietly published an administrative endpoint would
# make it an accident instead.
if grep -q '^kind: HTTPRoute$' "$output_file"; then
  printf 'The admin API route must be disabled by default\n' >&2
  exit 1
fi

# ...and it renders when asked for, attached to the platform Gateway.
routed=$(render --set admin.api.route.enabled=true --set admin.api.route.hostname=admin-api.test.invalid)
grep -q '^kind: HTTPRoute$' <<<"$routed"
grep -q 'sectionName: websecure' <<<"$routed"
grep -q 'admin-api.test.invalid' <<<"$routed"

# A managed service is served only when configured, and its credential must come
# from a Secret rather than from values — a connection string in a values file
# would carry a superuser password into Git.
if grep -q 'ADMIN_POSTGRES_DSN\|ADMIN_MONGO_URI' "$output_file"; then
  printf 'Database credentials must not render unless the service is enabled\n' >&2
  exit 1
fi

configured=$(render --set admin.api.postgres.enabled=true --set admin.api.mongo.enabled=true)
grep -q 'name: ADMIN_POSTGRES_DSN' <<<"$configured"
grep -q 'name: ADMIN_MONGO_URI' <<<"$configured"
grep -q 'secretKeyRef' <<<"$configured"
if grep -qE 'ADMIN_(POSTGRES_DSN|MONGO_URI)$' -A 1 <<<"$configured" | grep -q '^ *value:'; then
  printf 'A database connection string was rendered as a literal value\n' >&2
  exit 1
fi

printf 'Admin chart validation passed.\n'
