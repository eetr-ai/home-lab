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

# The same chart with nothing but its own values.yaml, plus the four settings that
# are render failures when unset. Everything else keeps the shipped default, which
# is what makes an assertion about a default an assertion about the default.
render_defaults() {
  helm template home-lab-admin "${repo_root}/charts/admin" \
    --namespace admin \
    --kube-version "$kube_version" \
    --set admin.api.oidc.issuer=https://auth.test.invalid \
    --set admin.api.image.tag=0.0.1 \
    --set admin.web.hostname=admin.test.invalid \
    --set admin.web.clientId=test-client \
    --set admin.web.image.tag=0.0.1
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
if render --set admin.api.replicas=1 2>/dev/null | grep -q 'kind: PodDisruptionBudget'; then
  printf 'No PodDisruptionBudget may be emitted at one replica; it would deadlock a drain\n' >&2
  exit 1
fi

# THE WEB STAYS AT ONE, and this is the assertion most worth having here, because
# raising it looks like an improvement and is an outage. eetr-auth rotates refresh
# tokens with reuse detection: two replicas can present the same one, and the
# answer is the whole family revoked and the operator signed out. The single-flight
# that prevents it is an in-process Map in src/lib/auth/refresh.ts, so it reaches
# exactly one pod. Raising this needs that lock moved behind platform.redis first.
if grep -qE '^  replicas: [2-9]' <<<"$web_pod"; then
  printf 'admin-web must run exactly one replica until its token refresh takes a shared lock\n' >&2
  exit 1
fi
grep -q '^  replicas: 1$' <<<"$web_pod"

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
