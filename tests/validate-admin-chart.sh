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
if grep -E -A 1 'ADMIN_(POSTGRES_DSN|MONGO_URI)$' <<<"$configured" | grep -q '^ *value:'; then
  printf 'A database connection string was rendered as a literal value\n' >&2
  exit 1
fi

# The panel reads the cluster and does not change it. This is the assertion that
# holds that true, so it reads the rendered document rather than grepping the whole
# output: a write verb anywhere in the release would otherwise be invisible, and an
# unrelated ClusterRoleBinding would satisfy a bare `grep -q`.
rbac=$(render --show-only templates/rbac.yaml)
release_role='home-lab-admin-read'

# Every granted verb, normalised out of whichever YAML style the template used —
# flow (`verbs: ["get"]`) or block (`verbs:` then `- get`). Matching the text as
# written would pass the moment somebody reformatted the template.
granted_verbs=$(printf '%s\n' "$rbac" | awk '
  /^[[:space:]]*verbs:[[:space:]]*\[/ { line=$0; sub(/^[^[]*\[/, "", line); sub(/\].*$/, "", line); print line; next }
  /^[[:space:]]*verbs:[[:space:]]*$/   { block=1; next }
  block && /^[[:space:]]*-[[:space:]]/ { line=$0; sub(/^[[:space:]]*-[[:space:]]*/, "", line); print line; next }
  block                                { block=0 }
' | tr -d '"'"'"',' | tr ' ' '\n' | sed '/^[[:space:]]*$/d' | sort -u)

if [[ -z $granted_verbs ]]; then
  printf 'No verbs found in the rendered ClusterRole. The assertion below would pass on anything.\n' >&2
  exit 1
fi

while read -r verb; do
  case "$verb" in
    get | list | watch) ;;
    *)
      printf 'The admin ClusterRole must be read-only; it grants %s\n' "$verb" >&2
      exit 1
      ;;
  esac
done <<<"$granted_verbs"

# The resources are pinned too, not just the verbs. Every one of these is read by
# a named method in admin/api/internal/kube; a standing grant for anything else is
# a permission nobody is reviewing. Add one back in the change that reads it.
granted_resources=$(printf '%s\n' "$rbac" | awk '
  /^[[:space:]]*resources:[[:space:]]*$/   { block=1; next }
  block && /^[[:space:]]*-[[:space:]]/     { line=$0; sub(/^[[:space:]]*-[[:space:]]*/, "", line); print line; next }
  block                                    { block=0 }
' | tr -d '"' | sort -u | tr '\n' ' ')
expected_resources='daemonsets deployments events namespaces pods statefulsets '
if [[ $granted_resources != "$expected_resources" ]]; then
  printf 'The admin ClusterRole grants [%s]; expected [%s].\n' "$granted_resources" "$expected_resources" >&2
  printf 'If the API now reads something new, add it here in the same change.\n' >&2
  exit 1
fi

# The binding has to name this release's role and this release's ServiceAccount.
# Checking only that *a* ClusterRoleBinding exists would pass on one pointing
# somewhere else entirely.
binding=$(printf '%s\n' "$rbac" | awk '/^kind: ClusterRoleBinding$/,0')

# roleRef is read as its own block rather than grepped out of the whole binding.
# The binding's metadata carries the same name, so a document-wide grep for the
# role name is satisfied by that — and passes while roleRef points at
# cluster-admin. Mutation-checked; it did exactly that before this was scoped.
role_ref=$(printf '%s\n' "$binding" | awk '/^roleRef:/{inref=1; next} /^[^[:space:]]/{inref=0} inref')
grep -q "^  kind: ClusterRole$" <<<"$role_ref"
if ! grep -q "^  name: ${release_role}$" <<<"$role_ref"; then
  printf 'The ClusterRoleBinding does not point at %s:\n%s\n' "$release_role" "$role_ref" >&2
  exit 1
fi

subjects=$(printf '%s\n' "$binding" | awk '/^subjects:/{insub=1; next} /^[^[:space:]]/{insub=0} insub')
grep -q 'kind: ServiceAccount' <<<"$subjects"
grep -q 'name: admin-api$' <<<"$subjects"
grep -q 'namespace: admin$' <<<"$subjects"

# ...and disabling the cluster section removes both, rather than leaving a standing
# grant for endpoints that are not served.
disabled=$(render --set admin.api.kubernetes.enabled=false)
if grep -q "name: ${release_role}$" <<<"$disabled"; then
  printf 'Disabling the Kubernetes section must not leave %s behind\n' "$release_role" >&2
  exit 1
fi

printf 'Admin chart validation passed.\n'
