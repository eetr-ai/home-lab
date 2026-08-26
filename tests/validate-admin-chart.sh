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

# The panel is routable — a panel nobody can reach is not a panel — and the API
# is not. That asymmetry is the point: the browser-facing half is meant to be
# published (behind Cloudflare Access), while exposing the administrative API
# itself is a separate decision taken when something needs to call it.
routes=$(awk '/^kind: HTTPRoute$/,/^---$/' "$output_file")
grep -q 'name: admin-web' <<<"$routes"
if grep -q 'name: admin-api' <<<"$routes"; then
  printf 'The admin API route must be disabled by default\n' >&2
  exit 1
fi
grep -q 'sectionName: websecure' <<<"$routes"

# ...and the API route renders when asked for, attached to the same Gateway.
routed=$(render --set admin.api.route.enabled=true --set admin.api.route.hostname=admin-api.test.invalid)
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
rbac=$(render --show-only templates/api/rbac.yaml)
release_role='home-lab-admin-read'

# Every (apiGroup/resource, verb) the ClusterRole grants, scoped to the rule that
# granted it. The pairing is the whole point: a flat set of verbs cannot tell
# `get` on nodes from `get` on nodes/proxy, and the day this role holds a write
# verb it could not tell `patch` on deployments/scale from `patch` on secrets.
#
# Both YAML styles the template might use are parsed — flow (`verbs: ["get"]`) and
# block (`verbs:` then `- get`) — so a reformat does not quietly change what is
# asserted. Parsing stops at the ClusterRoleBinding, which has no rules of its own
# but would otherwise contribute to the set.
extract_pairs() {
  awk '
    function items(line,   s) { s = line; sub(/^[^[]*\[/, "", s); sub(/\].*$/, "", s)
        gsub(/""/, "core", s); gsub(/[",]/, " ", s); return s }
    function push(arr, s,   i, k, n, parts) { n = 0; k = split(s, parts, /[ \t]+/)
        for (i = 1; i <= k; i++) if (parts[i] != "") arr[++n] = parts[i]; return n }
    function flush(   g, r, v) {
      for (g = 1; g <= ng; g++) for (r = 1; r <= nr; r++) for (v = 1; v <= nv; v++)
        printf "%s/%s %s\n", grp[g], res[r], vrb[v]
      ng = 0; nr = 0; nv = 0
    }
    /^kind: ClusterRoleBinding$/ { flush(); stop = 1 }
    stop { next }
    /^[[:space:]]*-[[:space:]]*apiGroups:[[:space:]]*\[/ {
        flush(); mode = ""; ng = push(grp, items($0)); next }
    /^[[:space:]]*-[[:space:]]*apiGroups:[[:space:]]*$/ { flush(); mode = "g"; ng = 0; next }
    /^[[:space:]]*resources:[[:space:]]*\[/ { mode = ""; nr = push(res, items($0)); next }
    /^[[:space:]]*verbs:[[:space:]]*\[/     { mode = ""; nv = push(vrb, items($0)); next }
    /^[[:space:]]*apiGroups:[[:space:]]*$/  { mode = "g"; ng = 0; next }
    /^[[:space:]]*resources:[[:space:]]*$/  { mode = "r"; nr = 0; next }
    /^[[:space:]]*verbs:[[:space:]]*$/      { mode = "v"; nv = 0; next }
    mode != "" && /^[[:space:]]*-[[:space:]]/ {
        s = $0; sub(/^[[:space:]]*-[[:space:]]*/, "", s); gsub(/"/, "", s)
        if (s == "") s = "core"
        if (mode == "g") grp[++ng] = s
        else if (mode == "r") res[++nr] = s
        else vrb[++nv] = s
        next }
    { mode = "" }
    END { flush() }
  ' | LC_ALL=C sort -u
}

# LC_ALL=C on both sides is not cosmetic: nodes/proxy contains a slash, and the
# default collation on glibc ignores punctuation at the first level — so the two
# sides could sort differently on a laptop and in CI.
granted_pairs=$(printf '%s\n' "$rbac" | extract_pairs)

if [[ -z $granted_pairs ]]; then
  printf 'No rules found in the rendered ClusterRole. The assertion below would pass on anything.\n' >&2
  exit 1
fi

# Every pair, spelled out. Each is exercised by a named method in
# admin/api/internal/kube; a standing grant for anything else is a permission
# nobody is reviewing. Add one back in the change that reads it.
expected_pairs=$(LC_ALL=C sort -u <<'PAIRS'
apps/daemonsets get
apps/daemonsets list
apps/daemonsets watch
apps/deployments get
apps/deployments list
apps/deployments watch
apps/statefulsets get
apps/statefulsets list
apps/statefulsets watch
core/events get
core/events list
core/events watch
core/namespaces get
core/namespaces list
core/namespaces watch
core/nodes get
core/nodes list
core/nodes watch
core/persistentvolumeclaims get
core/persistentvolumeclaims list
core/persistentvolumeclaims watch
core/persistentvolumes get
core/persistentvolumes list
core/persistentvolumes watch
core/pods get
core/pods list
core/pods watch
metrics.k8s.io/nodes get
metrics.k8s.io/nodes list
metrics.k8s.io/pods get
metrics.k8s.io/pods list
PAIRS
)

if [[ $granted_pairs != "$expected_pairs" ]]; then
  printf 'The admin ClusterRole grants a different set of permissions than expected.\n' >&2
  diff <(printf '%s\n' "$expected_pairs") <(printf '%s\n' "$granted_pairs") >&2 || true
  printf 'If the API now reads something new, add the pair here in the same change.\n' >&2
  exit 1
fi

# Belt and braces, so a mistake in the list above is still caught. These verbs are
# never right for this role however they are scoped: the panel reads the cluster
# and creates, deletes, changes, or grants nothing in it.
forbidden_verbs=' (create|update|patch|delete|deletecollection|bind|escalate|impersonate|\*)$'
if grep -qE "$forbidden_verbs" <<<"$granted_pairs"; then
  printf 'The admin ClusterRole grants a verb it must never hold:\n' >&2
  grep -E "$forbidden_verbs" <<<"$granted_pairs" >&2
  exit 1
fi

# ...and no wildcard on either side of a rule.
if grep -qE '(^\*/|/\*)' <<<"$granted_pairs"; then
  printf 'The admin ClusterRole uses a wildcard group or resource\n' >&2
  exit 1
fi

# Reading node disk usage means reaching the kubelet through the node proxy, which
# also opens its other read endpoints. That is a deliberate choice, so assert both
# halves: it is absent by default, and it appears — with `get` and nothing else —
# only when it is asked for.
if grep -q 'nodes/proxy' <<<"$granted_pairs"; then
  printf 'The nodes/proxy grant must not be held by default\n' >&2
  exit 1
fi

node_stats_pairs=$(render --set admin.api.kubernetes.nodeStats.enabled=true \
  --show-only templates/api/rbac.yaml | extract_pairs | grep 'nodes/proxy' || true)
if [[ $node_stats_pairs != 'core/nodes/proxy get' ]]; then
  printf 'Enabling node stats must grant get on nodes/proxy and nothing else; it granted [%s]\n' \
    "$node_stats_pairs" >&2
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

# The panel's own half. It is an OIDC client, so both of its secrets come from a
# Secret rather than from values — a client secret or a cookie-sealing key in a
# values file would end up in Git, and this repository is public.
grep -q 'name: admin-web' "$output_file"
for variable in AUTH_SECRET OIDC_CLIENT_SECRET; do
  grep -q "name: $variable" "$output_file"
  if grep -A 1 "name: $variable\$" "$output_file" | grep -q '^ *value:'; then
    printf '%s was rendered as a literal value rather than read from a Secret\n' "$variable" >&2
    exit 1
  fi
done

# One document, not a grep across the whole render: several resources carry the
# name admin-web, and an assertion that matched any of them would pass on the
# Service while the Deployment was wrong.
web_pod=$(render --show-only templates/web/deployment.yaml)

# The panel asks the API for everything and never reads the Kubernetes API itself,
# so it must carry no ServiceAccount token. Without this the browser-facing pod
# would hold a cluster credential it has no use for.
grep -q 'automountServiceAccountToken: false' <<<"$web_pod"

# AUTH_URL is derived from the hostname rather than configured separately: Auth.js
# builds the OAuth callback from it, and a value that disagrees with the hostname
# fails at the callback — long after the mistake was made.
grep -q 'value: "https://admin.example.invalid"' <<<"$web_pod"

# Both halves of the chart are unusable without them, so each is a render failure
# rather than a pod that starts and cannot sign anybody in.
if render --set admin.web.hostname= >/dev/null 2>&1; then
  printf 'The chart rendered with no panel hostname\n' >&2
  exit 1
fi

if render --set admin.web.clientId= >/dev/null 2>&1; then
  printf 'The chart rendered with no OIDC client id\n' >&2
  exit 1
fi

printf 'Admin chart validation passed.\n'
