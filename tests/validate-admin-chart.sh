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

# The same chart with nothing but its own values.yaml, plus the three settings that
# are render failures when unset. Everything else keeps the shipped default, which
# is what makes an assertion about a default an assertion about the default.
#
# No image tag among them: it is not a render failure any more, because it
# defaults to the chart's appVersion.
render_defaults() {
  helm template home-lab-admin "${repo_root}/charts/admin" \
    --namespace admin \
    --kube-version "$kube_version" \
    --set admin.api.oidc.issuer=https://auth.test.invalid \
    --set admin.web.hostname=admin.test.invalid \
    --set admin.web.clientId=test-client
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

# One tag reaches all three images.
#
# Asserted on the rendered tags rather than on the values, because what matters is
# that nothing renders a second one: the whole point of the single key is that a
# mismatched set is unrepresentable. Read as the count of DISTINCT tags across
# every image in the document, so a template that grew its own tag key fails here
# even though it would render perfectly well.
rendered_tags=$(grep -E '^ +image: ' "$output_file" |
  sed -E 's/.*:([^:"]+)"?$/\1/' | sort -u)
if [[ $(wc -l <<<"$rendered_tags") -ne 1 ]]; then
  printf 'Every admin image must carry admin.image.tag; found several:\n%s\n' \
    "$rendered_tags" >&2
  exit 1
fi

# The per-component tags are gone, not defaulted. Asserted by rendering with one,
# because a values file still carrying it must fail loudly rather than be quietly
# ignored -- $defs/image is additionalProperties: false, and that is what turns
# this migration into a message naming the stale key.
if render --set admin.api.image.tag=0.0.2 >/dev/null 2>&1; then
  printf 'admin.api.image.tag is gone; setting it must fail the render rather\n' >&2
  printf 'than being ignored.\n' >&2
  exit 1
fi

# The tag defaults to the chart's appVersion, which is what lets a values file name
# no version at all and still install the images the chart was released with.
#
# Asserted against a PACKAGED chart rather than the working tree, because that is
# the only way to control appVersion -- and packaging is what a release actually
# does. A chart packaged at 9.9.9 must run 9.9.9 images with nothing in values
# saying so.
package_dir=$(mktemp -d)
trap 'rm -rf "$package_dir"' EXIT
helm package "${repo_root}/charts/admin" \
  --version 9.9.9 --app-version 9.9.9 --destination "$package_dir" >/dev/null
packaged=$(helm template home-lab-admin "${package_dir}/home-lab-admin-9.9.9.tgz" \
  --namespace admin \
  --kube-version "$kube_version" \
  --set admin.api.oidc.issuer=https://auth.test.invalid \
  --set admin.web.hostname=admin.test.invalid \
  --set admin.web.clientId=test-client)
if grep -qE '^ +image: .*:9\.9\.9"?$' <<<"$packaged"; then
  :
else
  printf 'The image tag must default to the chart appVersion\n' >&2
  exit 1
fi
if grep -E '^ +image: ' <<<"$packaged" | grep -qv ':9\.9\.9"'; then
  printf 'Every image must take the appVersion default, not only some\n' >&2
  exit 1
fi

# ...and an explicit tag still wins over it, which is what makes the default a
# default rather than a hard-coding.
helm template home-lab-admin "${package_dir}/home-lab-admin-9.9.9.tgz" \
  --namespace admin \
  --kube-version "$kube_version" \
  --set admin.api.oidc.issuer=https://auth.test.invalid \
  --set admin.web.hostname=admin.test.invalid \
  --set admin.web.clientId=test-client \
  --set admin.image.tag=8.8.8 | grep -q ':8\.8\.8"'

# "latest" arriving through appVersion must be refused too.
#
# values.schema.json cannot catch this one: it validates values, and this tag
# never appears in them. Without the check in _helpers.tpl a chart packaged with
# --app-version latest would render a moving tag past every guard in this file.
helm package "${repo_root}/charts/admin" \
  --version 9.9.10 --app-version latest --destination "$package_dir" >/dev/null
if helm template home-lab-admin "${package_dir}/home-lab-admin-9.9.10.tgz" \
  --namespace admin \
  --kube-version "$kube_version" \
  --set admin.api.oidc.issuer=https://auth.test.invalid \
  --set admin.web.hostname=admin.test.invalid \
  --set admin.web.clientId=test-client >/dev/null 2>&1; then
  printf 'A chart packaged with --app-version latest must not render\n' >&2
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

# The query console runs as the credential above and has no value of its own.
# A reappearing second DSN would mean the console had been split off again.
if grep -q 'ADMIN_POSTGRES_QUERY_DSN' <<<"$configured"; then
  printf 'The query console takes no separate credential\n' >&2
  exit 1
fi

# What the panel may do to the cluster. This is the assertion that holds it to
# exactly that, so it reads the rendered document rather than grepping the whole
# output: a grant anywhere in the release would otherwise be invisible, and an
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
# One YAML document out of a multi-document render, chosen by metadata.name.
#
# rbac-deploy.yaml renders two roles now — one the API reads with, one the Job
# deploys with — and reading the whole file merges their rules into a single set.
# That would pass on a chart that had given the API every write verb back, which
# is the exact regression these assertions exist to catch.
doc_named() {
  awk -v want="  name: $1" '
    /^---$/ { if (match_found) exit; buffer = ""; match_found = 0; next }
    { buffer = buffer $0 "\n" }
    $0 == want { match_found = 1 }
    END { if (match_found) printf "%s", buffer }
  '
}

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
apps/deployments patch
apps/deployments watch
apps/deployments/scale get
apps/deployments/scale update
apps/statefulsets get
apps/statefulsets list
apps/statefulsets patch
apps/statefulsets watch
apps/statefulsets/scale get
apps/statefulsets/scale update
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
core/pods/log get
core/services get
core/services list
core/services watch
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

# Belt and braces, so a mistake in the expected list above is still caught. These
# verbs are never right for this role however they are scoped: the panel rolls and
# resizes what already exists, and brings nothing into being, takes nothing away,
# and grants nobody anything.
#
# `patch` and `update` are not here — they are legitimate now, on exactly the four
# pairs listed above — which is the whole reason the assertion had to become pairs
# rather than a flat set of verbs. It could no longer tell `patch` on
# deployments/scale from `patch` on secrets.
forbidden_verbs=' (create|delete|deletecollection|bind|escalate|impersonate|\*)$'
if grep -qE "$forbidden_verbs" <<<"$granted_pairs"; then
  printf 'The admin ClusterRole grants a verb it must never hold:\n' >&2
  grep -E "$forbidden_verbs" <<<"$granted_pairs" >&2
  exit 1
fi

# ...and the write verbs it does hold reach only the four workload pairs. Spelled
# out separately from the list above so that widening that list to, say, secrets
# still trips this — the two assertions have to be wrong in the same way to pass.
writable=$(grep -E ' (patch|update)$' <<<"$granted_pairs" | LC_ALL=C sort)
expected_writable=$(LC_ALL=C sort <<'WRITABLE'
apps/deployments patch
apps/deployments/scale update
apps/statefulsets patch
apps/statefulsets/scale update
WRITABLE
)
if [[ $writable != "$expected_writable" ]]; then
  printf 'The admin ClusterRole may write to something unexpected.\n' >&2
  diff <(printf '%s\n' "$expected_writable") <(printf '%s\n' "$writable") >&2 || true
  exit 1
fi

# ...and no wildcard on either side of a rule.
if grep -qE '(^\*/|/\*)' <<<"$granted_pairs"; then
  printf 'The admin ClusterRole uses a wildcard group or resource\n' >&2
  exit 1
fi

# Creating and deleting namespaces is the widest write this ClusterRole can hold,
# so assert both halves: absent by default, and exactly two verbs when asked for.
#
# The forbidden-verbs assertion above already fails on a create or delete leaking
# into the default render, which is why this one only has to prove that turning it
# on grants these two pairs and nothing else. The two have to be wrong in the same
# way to pass.
if grep -q 'namespaces create' <<<"$granted_pairs"; then
  printf 'Namespace management must not be granted by default\n' >&2
  exit 1
fi

# The whole rendered set, not just the write verbs. Filtering to create/delete
# would let a `get` or a `patch` added to the same block through unseen, which is
# the mistake this file has made before: an assertion that only looks where it
# expects trouble is an assertion that stops finding it.
manage_pairs=$(render --set admin.api.kubernetes.namespaces.manage=true \
  --show-only templates/api/rbac.yaml | extract_pairs)
expected_manage_pairs=$(printf '%s\ncore/namespaces create\ncore/namespaces delete\n' \
  "$expected_pairs" | grep -v '^$' | LC_ALL=C sort)
if [[ $manage_pairs != "$expected_manage_pairs" ]]; then
  printf 'Enabling namespace management must add create and delete on namespaces, and nothing else.\n' >&2
  diff <(printf '%s\n' "$expected_manage_pairs") <(printf '%s\n' "$manage_pairs") >&2 || true
  exit 1
fi

# The panel refuses to delete the namespace it is running in, and it learns which
# one that is from the downward API. A literal here would silently stop matching
# the day the chart is installed under another release name -- and the failure
# would be the panel deleting itself.
if ! grep -A3 'name: POD_NAMESPACE' "$output_file" | grep -q 'fieldPath: metadata.namespace'; then
  printf 'POD_NAMESPACE must come from the downward API, not from a literal\n' >&2
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

# The Helm read grant, which is the widest thing this chart can be asked to
# create. Reading a release means reading every Secret in its namespace, so the
# assertions here are about containment rather than about the grant itself.
#
# Nothing at all by default: an empty namespace list must leave no Role, no
# RoleBinding, and no reference to one behind.
if grep -qE '^kind: Role$|-helm(-read|-jobs)?$|admin-helm-job' "$output_file"; then
  printf 'The Helm grants must not be created by default\n' >&2
  exit 1
fi

helm_rbac=$(render --set 'admin.api.helm.namespaces[0]=apps' \
  --show-only templates/api/rbac-deploy.yaml)

# A Role, never a ClusterRole. That is the containment, and it is the failure that
# matters: a ClusterRole here would grant read on every Secret in the cluster.
if grep -q '^kind: ClusterRole' <<<"$helm_rbac"; then
  printf 'The Helm grant must be a namespaced Role, never a ClusterRole\n' >&2
  exit 1
fi
# Two of each per namespace: one pair the API reads releases with, one pair the
# Job deploys with. The split is the point of the whole arrangement, so a render
# that collapsed back to one is a failure whichever one survived.
if [[ $(grep -c '^kind: Role$' <<<"$helm_rbac") != 2 ]] ||
   [[ $(grep -c '^kind: RoleBinding$' <<<"$helm_rbac") != 2 ]]; then
  printf 'One managed namespace must render two Roles and two RoleBindings\n' >&2
  exit 1
fi

# THE assertion of this change: the deploy grant belongs to the Job's account and
# not to the API's.
#
# Read the binding's own subjects block rather than grepping the document. A
# document-wide grep for admin-helm-job passes on a chart that ALSO still binds
# admin-api, which is precisely the regression that would silently undo this.
deploy_subjects=$(printf '%s\n' "$helm_rbac" |
  awk '/^kind: RoleBinding$/ { inside = 0 } /name: home-lab-admin-helm$/ { inside = 1 }
       inside && /^subjects:/ { grab = 1; next } grab && /^[^ ]/ { grab = 0 }
       grab { print }')
if ! grep -q 'name: admin-helm-job' <<<"$deploy_subjects"; then
  printf 'The Helm deploy grant must be bound to admin-helm-job\n' >&2
  exit 1
fi
if grep -q 'name: admin-api' <<<"$deploy_subjects"; then
  printf 'The Helm deploy grant must NOT be bound to admin-api: it is the long-lived\n' >&2
  printf 'credential two pods carry a token for, and moving it off that account is\n' >&2
  printf 'the entire point of running Helm in a Job.\n' >&2
  exit 1
fi

# The API keeps exactly enough to READ a release, which means Secrets and only
# Secrets, and only get/list/watch. A write verb here would put the deploy grant
# back on the long-lived credential one resource at a time.
helm_read_pairs=$(printf '%s\n' "$helm_rbac" | doc_named home-lab-admin-helm-read | extract_pairs)
expected_read_pairs=$(LC_ALL=C sort <<'READPAIRS'
core/secrets get
core/secrets list
core/secrets watch
READPAIRS
)
if [[ $helm_read_pairs != "$expected_read_pairs" ]]; then
  printf 'The API Helm read grant must be Secrets get/list/watch and nothing else.\n' >&2
  diff <(printf '%s\n' "$expected_read_pairs") <(printf '%s\n' "$helm_read_pairs") >&2 || true
  exit 1
fi

# ...bound into the namespace it names, not into the release namespace.
if ! grep -A4 '^kind: Role$' <<<"$helm_rbac" | grep -q '  namespace: apps'; then
  printf 'The Helm Role must live in the namespace it grants access to\n' >&2
  exit 1
fi

# The Job's own grant: everything a chart in this lab needs to be installed.
# Scoped to that one role, because the document also holds the API's read role and
# reading both together would merge their rules.
helm_pairs=$(printf '%s\n' "$helm_rbac" | doc_named home-lab-admin-helm | extract_pairs)
expected_helm_pairs=$(LC_ALL=C sort <<'HELMPAIRS'
rbac.authorization.k8s.io/clusterrolebindings create
rbac.authorization.k8s.io/clusterrolebindings delete
rbac.authorization.k8s.io/clusterrolebindings get
rbac.authorization.k8s.io/clusterrolebindings list
rbac.authorization.k8s.io/clusterrolebindings patch
rbac.authorization.k8s.io/clusterrolebindings update
rbac.authorization.k8s.io/clusterrolebindings watch
rbac.authorization.k8s.io/clusterroles create
rbac.authorization.k8s.io/clusterroles delete
rbac.authorization.k8s.io/clusterroles get
rbac.authorization.k8s.io/clusterroles list
rbac.authorization.k8s.io/clusterroles patch
rbac.authorization.k8s.io/clusterroles update
rbac.authorization.k8s.io/clusterroles watch
rbac.authorization.k8s.io/rolebindings create
rbac.authorization.k8s.io/rolebindings delete
rbac.authorization.k8s.io/rolebindings get
rbac.authorization.k8s.io/rolebindings list
rbac.authorization.k8s.io/rolebindings patch
rbac.authorization.k8s.io/rolebindings update
rbac.authorization.k8s.io/rolebindings watch
rbac.authorization.k8s.io/roles create
rbac.authorization.k8s.io/roles delete
rbac.authorization.k8s.io/roles get
rbac.authorization.k8s.io/roles list
rbac.authorization.k8s.io/roles patch
rbac.authorization.k8s.io/roles update
rbac.authorization.k8s.io/roles watch
apps/daemonsets create
apps/daemonsets delete
apps/daemonsets get
apps/daemonsets list
apps/daemonsets patch
apps/daemonsets update
apps/daemonsets watch
apps/deployments create
apps/deployments delete
apps/deployments get
apps/deployments list
apps/deployments patch
apps/deployments update
apps/deployments watch
apps/replicasets get
apps/replicasets list
apps/replicasets watch
apps/statefulsets create
apps/statefulsets delete
apps/statefulsets get
apps/statefulsets list
apps/statefulsets patch
apps/statefulsets update
apps/statefulsets watch
batch/cronjobs create
batch/cronjobs delete
batch/cronjobs get
batch/cronjobs list
batch/cronjobs patch
batch/cronjobs update
batch/cronjobs watch
batch/jobs create
batch/jobs delete
batch/jobs get
batch/jobs list
batch/jobs patch
batch/jobs update
batch/jobs watch
core/configmaps create
core/configmaps delete
core/configmaps get
core/configmaps list
core/configmaps patch
core/configmaps update
core/configmaps watch
core/persistentvolumeclaims create
core/persistentvolumeclaims delete
core/persistentvolumeclaims get
core/persistentvolumeclaims list
core/persistentvolumeclaims patch
core/persistentvolumeclaims update
core/persistentvolumeclaims watch
core/secrets create
core/secrets delete
core/secrets get
core/secrets list
core/secrets patch
core/secrets update
core/secrets watch
core/serviceaccounts create
core/serviceaccounts delete
core/serviceaccounts get
core/serviceaccounts list
core/serviceaccounts patch
core/serviceaccounts update
core/serviceaccounts watch
core/services create
core/services delete
core/services get
core/services list
core/services patch
core/services update
core/services watch
gateway.networking.k8s.io/httproutes create
gateway.networking.k8s.io/httproutes delete
gateway.networking.k8s.io/httproutes get
gateway.networking.k8s.io/httproutes list
gateway.networking.k8s.io/httproutes patch
gateway.networking.k8s.io/httproutes update
gateway.networking.k8s.io/httproutes watch
networking.k8s.io/networkpolicies create
networking.k8s.io/networkpolicies delete
networking.k8s.io/networkpolicies get
networking.k8s.io/networkpolicies list
networking.k8s.io/networkpolicies patch
networking.k8s.io/networkpolicies update
networking.k8s.io/networkpolicies watch
policy/poddisruptionbudgets create
policy/poddisruptionbudgets delete
policy/poddisruptionbudgets get
policy/poddisruptionbudgets list
policy/poddisruptionbudgets patch
policy/poddisruptionbudgets update
policy/poddisruptionbudgets watch
HELMPAIRS
)
if [[ $helm_pairs != "$expected_helm_pairs" ]]; then
  printf 'The Helm Role grants something other than the reviewed set.\n' >&2
  printf 'Adding a resource here is a decision, not a fix: read rbac-deploy.yaml.\n' >&2
  diff <(printf '%s\n' "$expected_helm_pairs") <(printf '%s\n' "$helm_pairs") >&2 || true
  exit 1
fi

# ReplicaSets are read-only and must stay that way: nothing here creates one, and
# the grant exists only so Helm's readiness wait can walk down from a Deployment.
if grep -qE '^apps/replicasets (create|update|patch|delete)$' <<<"$helm_pairs"; then
  printf 'The Helm Role must not write ReplicaSets\n' >&2
  exit 1
fi

# Two groups it must never reach. Granting RBAC would let a chart hand the panel's
# ServiceAccount more than it holds; granting CRDs would let one change what the
# cluster's kinds mean. Checked against the granted pairs rather than the rendered
# text, because the RoleBinding's own roleRef legitimately names the RBAC group --
# grepping the document would fail on a correct chart, which is the kind of
# assertion that gets deleted rather than fixed.
# CustomResourceDefinitions stay out. A chart needing them is one to install by
# hand and then read from the panel.
#
# RBAC does not, and used to: admin.api.helm.selfDeploy gated it because these
# verbs were held by the API's own long-lived credential, which made "can hand
# every permission it holds to any ServiceAccount" a standing property of it. They
# belong to a Job that lives for one operation now, so the flag is gone and the
# grant is unconditional. This chart renders four RBAC objects, so upgrading the
# panel needs them.
if grep -q '^apiextensions\.k8s\.io/' <<<"$helm_pairs"; then
  printf 'The Helm deploy grant must not reach CustomResourceDefinitions\n' >&2
  exit 1
fi

# What must never appear, flag or no flag. Without escalate and bind, Kubernetes
# refuses to let this create a role holding more than the Job already holds --
# which is the difference between a wide grant and an unbounded one, and the only
# bound left on it.
if grep -qE ' (escalate|bind)$' <<<"$helm_pairs"; then
  printf 'The Helm deploy grant must never hold escalate or bind: that removes the\n' >&2
  printf 'only thing stopping it granting more than it holds.\n' >&2
  exit 1
fi

# The flag that used to gate the RBAC verbs is gone, not defaulted. Asserted by
# rendering with it, because a values file still setting it must fail loudly
# rather than be quietly ignored -- values.schema.json is additionalProperties:
# false, and that is what makes this a clear message instead of a silent no-op.
if render --set admin.api.helm.selfDeploy=true >/dev/null 2>&1; then
  printf 'admin.api.helm.selfDeploy is gone; setting it must fail the render rather\n' >&2
  printf 'than being ignored.\n' >&2
  exit 1
fi

# The API's own write grant, which is the whole of what its long-lived credential
# may change on the cluster.
#
# Read the warning at the top of rbac-jobs.yaml before reading this as small:
# `create` on a Job here is, in reach, equivalent to holding the deploy grant
# above, because Kubernetes does not check whether a Job's creator may run as the
# Job's ServiceAccount. What these assert is the shape, not a bound.
jobs_rbac=$(render --set 'admin.api.helm.namespaces[0]=apps' \
  --show-only templates/api/rbac-jobs.yaml)
helm_deploy_env=$(render --set 'admin.api.helm.namespaces[0]=apps' \
  --show-only templates/api/deployment.yaml)

# Namespaced, and to the release namespace. A ClusterRole here would let the panel
# start a privileged pod in any namespace on the cluster.
if grep -q '^kind: ClusterRole' <<<"$jobs_rbac"; then
  printf 'The API Jobs grant must be a namespaced Role, never a ClusterRole\n' >&2
  exit 1
fi
if ! grep -A4 '^kind: Role$' <<<"$jobs_rbac" | grep -q '  namespace: admin'; then
  printf "The API Jobs grant must live in the release namespace\n" >&2
  exit 1
fi

jobs_pairs=$(printf '%s\n' "$jobs_rbac" | extract_pairs)
expected_jobs_pairs=$(LC_ALL=C sort <<'JOBPAIRS'
batch/jobs create
batch/jobs get
batch/jobs list
batch/jobs watch
JOBPAIRS
)
if [[ $jobs_pairs != "$expected_jobs_pairs" ]]; then
  printf 'The API Jobs grant must be exactly create/get/list/watch on jobs.\n' >&2
  printf 'No delete, update, or patch: the API never edits a Job, the TTL reaps them,\n' >&2
  printf 'and delete would be a way to cancel an operation mid-apply and wedge a release.\n' >&2
  diff <(printf '%s\n' "$expected_jobs_pairs") <(printf '%s\n' "$jobs_pairs") >&2 || true
  exit 1
fi

# The account the Job runs as exists wherever the grant that names it does.
if ! render --set 'admin.api.helm.namespaces[0]=apps' \
    --show-only templates/api/serviceaccount.yaml | grep -q 'name: admin-helm-job'; then
  printf 'A managed namespace must render the admin-helm-job ServiceAccount\n' >&2
  exit 1
fi

# Which account a Helm Job runs as is read from the API's own environment and is
# never influenced by a request -- see buildJob. Asserted here because the chart
# is where the value comes from.
if ! grep -A1 'name: ADMIN_HELM_JOB_SERVICE_ACCOUNT' <<<"$helm_deploy_env" |
    grep -q 'value: "\?admin-helm-job'; then
  printf 'ADMIN_HELM_JOB_SERVICE_ACCOUNT must name admin-helm-job\n' >&2
  exit 1
fi

# The Job runs the same image as the API. The design rests on it being the same
# binary, so a values key that could point it elsewhere does not exist.
if ! grep -A1 'name: ADMIN_HELM_JOB_IMAGE$' <<<"$helm_deploy_env" | grep -q 'admin-api:'; then
  printf "ADMIN_HELM_JOB_IMAGE must be the API's own image\n" >&2
  exit 1
fi

# The Job's pod carries requests, like every other workload here. Without them it
# is BestEffort, and BestEffort is the first thing evicted under node pressure --
# which here means killing the pod holding a release half-applied.
if ! grep -A1 'name: ADMIN_HELM_JOB_RESOURCES' <<<"$helm_deploy_env" | grep -q 'requests'; then
  printf 'The Helm job must carry resource requests: an unrequested pod is BestEffort,\n' >&2
  printf 'and evicting one mid-apply leaves a release wedged.\n' >&2
  exit 1
fi

# The self-upgrade special case is gone, and so is the identity it needed.
if grep -q 'ADMIN_RELEASE_NAME' "$output_file"; then
  printf 'ADMIN_RELEASE_NAME is gone: nothing recognises its own release any more,\n' >&2
  printf 'because a Job is not replaced by the chart it applies.\n' >&2
  exit 1
fi

# ...but the release namespace is allowed, because deploying the panel's own chart
# from a pipeline is what this feature was asked for. Asserted rather than left
# implicit: it is one line away from being refused again by somebody tidying the
# protected list, and the failure would look like a chart bug rather than a
# policy change.
if ! render --set 'admin.api.helm.namespaces[0]=admin' \
    --show-only templates/api/rbac-deploy.yaml | grep -q '  namespace: admin'; then
  printf "The release's own namespace must be allowed as a Helm target\n" >&2
  exit 1
fi

# admin.api.helm.allNamespaces gives the containment above up on purpose, and what
# matters is that it is the ONLY thing that does: one ClusterRole, no Roles, and
# exactly the same verbs as the bounded mode. A wider grant hiding behind this
# flag would be the worst of both.
helm_all=$(render --set admin.api.helm.allNamespaces=true \
  --show-only templates/api/rbac-deploy.yaml)

if [[ $(grep -c '^kind: ClusterRole$' <<<"$helm_all") != 2 ]] ||
   [[ $(grep -c '^kind: ClusterRoleBinding$' <<<"$helm_all") != 2 ]]; then
  printf 'allNamespaces must render exactly two ClusterRoles and two ClusterRoleBindings\n' >&2
  exit 1
fi
if grep -qE '^kind: Role$|^kind: RoleBinding$' <<<"$helm_all"; then
  printf 'allNamespaces must render no per-namespace Roles\n' >&2
  exit 1
fi
if [[ $(printf '%s\n' "$helm_all" | doc_named home-lab-admin-helm | extract_pairs) != "$expected_helm_pairs" ]]; then
  printf 'The cluster-wide Helm grant must hold exactly the same verbs as the bounded one.\n' >&2
  diff <(printf '%s\n' "$expected_helm_pairs") \
    <(printf '%s\n' "$helm_all" | doc_named home-lab-admin-helm | extract_pairs) >&2 || true
  exit 1
fi

# Helm needs somewhere to write while it works -- it unpacks charts and keeps an
# OCI layer cache -- and the pod's root filesystem is read-only. This is the
# assertion that would have caught "mkdir /home/nonroot/.cache: read-only file
# system", which presents as every install failing for an unrelated-looking reason.
cache=$(render --show-only templates/api/deployment.yaml)
if ! grep -q 'name: helm-cache' <<<"$cache"; then
  printf 'The API needs a writable volume for the Helm cache\n' >&2
  exit 1
fi
if ! grep -A2 'name: helm-cache' <<<"$cache" | grep -q 'emptyDir'; then
  printf 'The Helm cache must be an emptyDir, not a claim: two replicas would fight over one\n' >&2
  exit 1
fi
if ! grep -A3 'name: helm-cache' <<<"$cache" | grep -q 'sizeLimit'; then
  printf 'The Helm cache emptyDir must be bounded\n' >&2
  exit 1
fi
# Both variables, and both pointing inside the mount. Helm's registry client
# reads XDG_CACHE_HOME directly rather than going through HELM_CACHE_HOME, so
# setting only the Helm one leaves OCI pulls writing to a read-only home.
for cache_var in HELM_CACHE_HOME XDG_CACHE_HOME; do
  if ! grep -A1 "name: $cache_var" <<<"$cache" | grep -q 'value: /helm/'; then
    printf '%s must point inside the writable Helm volume\n' "$cache_var" >&2
    exit 1
  fi
done
if ! grep -q 'readOnlyRootFilesystem: true' <<<"$cache"; then
  printf 'The root filesystem must stay read-only with the Helm cache mounted\n' >&2
  exit 1
fi

# The record of declared deployments: its credential is read from a Secret, and
# the DSN is never rendered as a literal. Same rule as every other credential in
# this chart, and the installer's pre-flight depends on the secretKeyRef shape.
if render | grep -q 'ADMIN_HELM_DSN'; then
  printf 'The Helm deployment store must not be configured by default\n' >&2
  exit 1
fi
helm_dsn=$(render --set admin.api.helm.postgres.enabled=true \
  --show-only templates/api/deployment.yaml)
if ! grep -A3 'name: ADMIN_HELM_DSN' <<<"$helm_dsn" | grep -q 'secretKeyRef'; then
  printf 'ADMIN_HELM_DSN must be read from a Secret\n' >&2
  exit 1
fi
# The Job gets the same credential as a name and a key rather than a value, so
# the API can write a secretKeyRef without ever handling the connection string. A
# Job is not a Secret: anything able to list Jobs can read every literal in one.
if ! grep -q 'name: ADMIN_HELM_DSN_SECRET_NAME' <<<"$helm_dsn" ||
   ! grep -q 'name: ADMIN_HELM_DSN_SECRET_KEY' <<<"$helm_dsn"; then
  printf 'The API needs the DSN Secret name and key so a Job can reference it\n' >&2
  exit 1
fi
if grep -A3 'name: ADMIN_HELM_DSN' <<<"$helm_dsn" | grep -qE '(postgres://|postgresql://)'; then
  printf 'A connection string must never be rendered as a literal\n' >&2
  exit 1
fi

# A protected namespace in the list is a render failure, not a warning. This is
# the mistake that turns a bounded grant into an unbounded one, and a warning in
# a Helm output nobody reads is not a control.
for forbidden in platform-system kube-system kube-flannel default; do
  if render --set "admin.api.helm.namespaces[0]=$forbidden" >/dev/null 2>&1; then
    printf 'The chart rendered with %s as a Helm-managed namespace\n' "$forbidden" >&2
    exit 1
  fi
done

# Helm reaches the cluster through the same credential the cluster slice does, so
# it cannot be served without it.
if ! render --set admin.api.kubernetes.enabled=false | grep -q 'ADMIN_HELM_DISABLED'; then
  printf 'Disabling the cluster slice must also disable Helm\n' >&2
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

# --- the agent ---------------------------------------------------------------
#
# It is rendered here because the example values enable it, which is what puts its
# Deployment, Service and PVC in front of kubeconform. Its being *off* by default
# is asserted against the shipped values.yaml rather than against this render.
if grep -q 'name: admin-agent' <<<"$(render_defaults)"; then
  printf 'The agent must be disabled by default\n' >&2
  exit 1
fi

# --- the API runs more than one, and the web deliberately does not -------------
#
# These two sit together because the difference between them is the whole point,
# and reading either alone invites "make them consistent".
api_pod=$(render --show-only templates/api/deployment.yaml)

# The API holds no state — the operator's token comes with each request, the
# cluster reads go through the pod's ServiceAccount — so a second replica is just
# more of the same. Two, so a drained node or an OOM kill does not take the panel's
# API with it.
grep -q '^  replicas: 2$' <<<"$api_pod"

# ...and it is spread across nodes only where the cluster can manage it. Required
# anti-affinity on a one-node cluster leaves the second replica Pending forever,
# which turns redundancy into an outage; this asserts the preferred form is what
# is emitted.
grep -q 'preferredDuringSchedulingIgnoredDuringExecution' <<<"$api_pod"
if grep -q 'requiredDuringSchedulingIgnoredDuringExecution' <<<"$api_pod"; then
  printf 'The API must not require anti-affinity; a single-node cluster would leave a replica Pending\n' >&2
  exit 1
fi

# A budget so a node drain cannot take both at once. It bounds voluntary
# disruption only, which is why it is not the reason the count is 2.
api_pdb=$(render --show-only templates/api/pdb.yaml)
grep -q 'kind: PodDisruptionBudget' <<<"$api_pdb"
grep -q 'minAvailable: 1' <<<"$api_pdb"

# ...and it is NOT emitted at one replica, which is the case that matters: a
# budget of minAvailable: 1 over a single replica can never be satisfied, so the
# drain hangs rather than proceeding.
# Scoped to the API's own template. Grepping the whole render would answer for
# whichever component still had a budget, which is how this check started passing
# for the wrong reason the moment the web grew one.
if render --set admin.api.replicas=1 --show-only templates/api/pdb.yaml >/dev/null 2>&1; then
  printf 'No API PodDisruptionBudget may be emitted at one replica; it would deadlock a drain\n' >&2
  exit 1
fi

# THE WEB MAY NOW RUN TWO, because its token refresh is coordinated — but only
# because it is. This pairing is the assertion worth having here: raising the
# replica count without the shared lock looks like an improvement and is an
# outage, since eetr-auth's reuse detection answers two replicas presenting the
# same refresh token by revoking the whole family.
grep -q '^  replicas: 2$' <<<"$web_pod"

# The coordination is wired to the pod, not merely available in the chart.
grep -q 'name: REDIS_URL' <<<"$web_pod"
web_redis_password=$(grep -A3 'name: REDIS_PASSWORD' <<<"$web_pod")
grep -q 'secretKeyRef' <<<"$web_redis_password"

# ...and more than one replica WITHOUT it is a render failure rather than a
# setting that quietly reintroduces the race. Asserted on the message, because a
# check that only asks whether helm exited non-zero would pass on any unrelated
# breakage in this chart.
web_unsafe=$(render --set admin.web.redis.enabled=false 2>&1 >/dev/null) && {
  printf 'Two web replicas without a shared refresh lock must be refused\n' >&2
  exit 1
}
if ! grep -q 'requires admin.web.redis.enabled' <<<"$web_unsafe"; then
  printf 'The refusal must name the setting that fixes it; got: %s\n' "$web_unsafe" >&2
  exit 1
fi

# ...and one replica without it is still perfectly legal, which is what makes the
# guard a statement about coordination rather than about Redis.
render --set admin.web.redis.enabled=false --set admin.web.replicas=1 >/dev/null

web_pdb=$(render --show-only templates/web/pdb.yaml)
grep -q 'kind: PodDisruptionBudget' <<<"$web_pdb"
grep -q 'minAvailable: 1' <<<"$web_pdb"
if render --set admin.web.replicas=1 --set admin.web.redis.enabled=false \
  --show-only templates/web/pdb.yaml >/dev/null 2>&1; then
  printf 'No web PodDisruptionBudget may be emitted at one replica\n' >&2
  exit 1
fi

agent_pod=$(render --show-only templates/agent/deployment.yaml)

# One replica, and not because nobody got round to raising it. The standalone Octo
# runtime holds its object store — the agent's memory — in the process and writes
# it back to this volume; two replicas would each answer from their own copy and
# each overwrite the other's file. Recreate is the second half of the same fact:
# the default RollingUpdate runs two pods for a few seconds.
grep -q '^  replicas: 1$' <<<"$agent_pod"
grep -q 'type: Recreate' <<<"$agent_pod"
if grep -qE '^  replicas: [2-9]' <<<"$agent_pod"; then
  printf 'The agent must run exactly one replica; see the comment on its Deployment\n' >&2
  exit 1
fi

# It holds a shell's worth of programs and reaches the network with curl, so it is
# given no cluster credential at all. What it reads of the cluster it reads through
# the API, as the operator who asked.
grep -q 'automountServiceAccountToken: false' <<<"$agent_pod"

# The provider key is the one secret this pod holds, and it comes from a Secret
# rather than from values — this repository is public.
#
# Asserted as the four lines it has to be rather than as "no literal value". A
# rejection-only check passes on an empty value, on a configMapKeyRef, and on a
# secretKeyRef naming something else entirely; this one only passes on the shape
# that actually works.
key_env=$(grep -A 4 'name: OPENROUTER_API_KEY$' <<<"$agent_pod")
grep -q 'valueFrom:' <<<"$key_env"
grep -q 'secretKeyRef:' <<<"$key_env"
grep -q 'name: admin-agent-llm' <<<"$key_env"
grep -q 'key: apiKey' <<<"$key_env"
if grep -q '^ *value:' <<<"$key_env"; then
  printf 'The OpenRouter key was rendered as a literal value rather than read from a Secret\n' >&2
  exit 1
fi

# Probes answer on the runtime's admin port, not on the chat route: the chat flow
# serves POST only, so a probe against it would fail a healthy pod.
#
# Each probe is read as its own block rather than grepped out of the whole pod. A
# document-wide grep for `port: admin` is satisfied by either probe having it,
# which passes while the other points somewhere that answers 405 — and a liveness
# probe wrong that way restarts a pod that is working.
grep -q 'containerPort: 39999' <<<"$agent_pod"
for probe in readinessProbe livenessProbe; do
  block=$(awk -v want="$probe" '
    $0 ~ "^ *" want ":" { depth = match($0, /[^ ]/); inprobe = 1; next }
    inprobe && match($0, /[^ ]/) <= depth { inprobe = 0 }
    inprobe { print }
  ' <<<"$agent_pod")
  if ! grep -q 'port: admin' <<<"$block"; then
    printf 'The agent %s must use the admin port; it reads [%s]\n' "$probe" "$block" >&2
    exit 1
  fi
  if ! grep -qE 'path: /(readyz|healthz)$' <<<"$block"; then
    printf 'The agent %s must probe /readyz or /healthz; it reads [%s]\n' "$probe" "$block" >&2
    exit 1
  fi
done

if grep -q 'path: /chat' <<<"$agent_pod"; then
  printf 'The agent must not be probed on its chat route; it serves POST only\n' >&2
  exit 1
fi

# The memory and the workspace are one volume, and it has to be the NFS class: the
# store is read from this directory at startup, so a pod that comes back on another
# node has to find it there.
agent_pvc=$(render --show-only templates/agent/pvc.yaml)
grep -q 'storageClassName: "nfs-client"' <<<"$agent_pvc"
grep -q 'ReadWriteMany' <<<"$agent_pvc"

# ...and the agent is reached only through the panel, which is where the operator
# is authenticated and where their token is attached. A route on the Gateway would
# be an unauthenticated way to spend the provider key.
if grep -q 'name: admin-agent' <<<"$routes"; then
  printf 'The agent must not be published on the Gateway\n' >&2
  exit 1
fi

# The panel is the only thing that reaches the agent, so it is the panel that has
# to know where it is — and it must not be told when there is nothing there, since
# that address is what the launcher reads to decide whether to render at all.
grep -q 'name: AGENT_URL' <<<"$(render --show-only templates/web/deployment.yaml)"
if grep -q 'name: AGENT_URL' <<<"$(render --set admin.agent.enabled=false)"; then
  printf 'The panel must not be given an agent address when no agent is deployed\n' >&2
  exit 1
fi

# A ClusterIP is reachable from every pod on the cluster, and this one answers
# anybody. The policy is what makes "only the panel reaches it" true rather than
# intended — so assert both halves: the chat port is restricted to admin-web, and
# the admin port is not. Probes come from the kubelet rather than from a pod, and a
# policy that restricted 39999 too would fail a healthy pod on any CNI that does
# not exempt host traffic.
policy=$(render --show-only templates/agent/networkpolicy.yaml)

# Each ingress rule flattened to one line, so the assertions below are about which
# rule carries what rather than about the order the rules happen to be written in.
#
# It records the *kind* of every peer and not just the labels on it, which is the
# distinction that matters: `namespaceSelector: {}` and `ipBlock: 0.0.0.0/0` each
# widen a rule to the whole cluster while carrying no name at all, so a check that
# only looked for `admin-web` would pass on a rule that also admitted everyone. A
# peer is counted the moment its kind appears; the labels under it are appended to
# that peer.
#
# A rule begins at four spaces and a dash. Note that the dash line CARRIES the
# first key — a rule reading `- from:` puts `from:` there and never on a line of
# its own — which is what an earlier version of this got wrong: it looked for
# `from:` at six spaces, found none, and reported every rule as having no peers.
rules() {
  awk '
    /^  ingress:/ { inrules = 1; next }
    !inrules { next }
    /^[^ ]/ || /^  [^ ]/ { if (line != "") print line; inrules = 0; next }
    /^    - / { if (line != "") print line; line = ""; infrom = ($0 ~ /-[[:space:]]+from:/); next }
    /^      from:/ { infrom = 1; next }
    /^      ports:/ { infrom = 0; next }
    infrom && /(podSelector|namespaceSelector|ipBlock):/ {
      kind = $0; sub(/^[^a-z]*/, "", kind); sub(/:.*$/, "", kind)
      line = line " peer:" kind; next }
    /port: / { sub(/^.*port: /, ""); line = line " port:" $0; next }
    # A value is required, so `matchLabels:` — a key with nothing after it — does
    # not append an empty token to the peer it introduces.
    infrom && /^ *[a-zA-Z0-9._\/-]+: [^ ]/ {
      v = $0; sub(/^.*: /, "", v); if (v != "") line = line "=" v; next }
    END { if (line != "") print line }
  ' <<<"$policy"
}

# The conversation is reachable from the panel and from nothing else: exactly one
# peer, and it is a podSelector naming admin-web. Asserted as the whole peer list
# rather than as "contains admin-web", so a second peer beside it fails.
chat_rule=$(rules | grep 'port:8080')
chat_peers=$(grep -o 'peer:[^ ]*' <<<"$chat_rule" | tr '\n' ' ' | sed 's/ $//')
if [[ $chat_peers != 'peer:podSelector=admin-web' ]]; then
  printf 'The agent chat port must be reachable from admin-web and nothing else; its peers are [%s]\n' \
    "$chat_peers" >&2
  exit 1
fi

# ...and the probe port is reachable from anywhere, which is not an oversight.
# Probes come from the kubelet on the node rather than from a pod, so a rule that
# named any peer here would fail a healthy pod on any CNI that does not exempt
# host traffic.
probe_rule=$(rules | grep 'port:39999')
if grep -q 'peer:' <<<"$probe_rule"; then
  printf 'The probe port must not name a peer; the rule is [%s]\n' "$probe_rule" >&2
  exit 1
fi

# Disabling it takes all three away rather than leaving a claim behind holding a
# volume nothing reads.
without_agent=$(render --set admin.agent.enabled=false)
if grep -q 'admin-agent' <<<"$without_agent"; then
  printf 'Disabling the agent must remove its Deployment, Service, claim and policy\n' >&2
  exit 1
fi

# The connector validates the reasoning effort when it starts, so an unlisted value
# is a pod that will not start. The schema is what turns that into a render failure.
if render --set admin.agent.reasoning=enthusiastic >/dev/null 2>&1; then
  printf 'The chart rendered with an unsupported reasoning effort\n' >&2
  exit 1
fi

# OpenRouter model ids are vendor-prefixed and there is no bare name, so a bare one
# is a 404 on the first call rather than a startup failure — the slowest possible
# way to find out.
if render --set admin.agent.model=deepseek-v4-flash >/dev/null 2>&1; then
  printf 'The chart rendered with a model id that is not vendor-prefixed\n' >&2
  exit 1
fi

printf 'Admin chart validation passed.\n'
