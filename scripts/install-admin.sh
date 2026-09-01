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

# ...and the platform's Redis admits only namespaces carrying this one. The panel
# needs it to coordinate its token refresh across replicas; without it the pods
# start, the NetworkPolicy drops the connection, and every refresh fails closed —
# which is safe, and looks exactly like an intermittent sign-out.
#
# Applied unconditionally rather than only when redis is enabled: a label on a
# namespace grants nothing on its own, since reaching Redis still needs the
# password, and making it conditional means the day somebody turns redis on is the
# day they discover this was skipped.
kubectl label namespace "$namespace" \
  home-lab.example/redis-access=true \
  --overwrite

rendered_chart=$(helm template "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file")

# Bootstrap-enrolled namespaces need the label as well as the bindings.
#
# Enrolment has two keys and they live in different places: the RoleBindings,
# which this chart renders for every namespace in admin.api.helm.namespaces, and
# the home-lab.example/helm-managed label, which says the namespace asked to be a
# target. The panel applies both when it enrols one itself. Nothing applied the
# label to a namespace the CHART enrolled, so the release namespace came up with
# its bindings in place and the panel reporting no enrolment at all -- which also
# means refusing to deploy into it, and deploying the panel's own chart from a
# pipeline is what the whole feature was asked for.
#
# Read out of the rendered chart rather than out of the values file, for the same
# reason the Secret checks below are: the chart is what the cluster acts on. A
# namespace that got a binding gets the label, and the two cannot drift.
bootstrap_namespaces=$(printf '%s\n' "$rendered_chart" | awk -v release="$release" '
  /^kind: RoleBinding$/ { in_binding = 1; name = ""; ns = ""; next }
  /^kind: / { in_binding = 0 }
  in_binding && $1 == "name:" && name == "" { name = $2; next }
  in_binding && $1 == "namespace:" && ns == "" { ns = $2 }
  in_binding && name == release "-secrets" && ns != "" { print ns; in_binding = 0 }
' | sort -u)

while read -r bootstrap_namespace; do
  [[ -n $bootstrap_namespace ]] || continue
  kubectl label namespace "$bootstrap_namespace" \
    home-lab.example/helm-managed=true \
    --overwrite
done <<<"$bootstrap_namespaces"

# Say out loud which images this will run.
#
# The tag is normally not in the values file at all now: it defaults to the
# chart's appVersion, which release-please keeps in step with the release. That is
# the right default and it removes a number nobody should have to repeat -- but it
# also means the version being installed is no longer visible in anything the
# operator typed. A checkout whose Chart.yaml had drifted would install the wrong
# build with nothing on screen saying so, which is exactly the class of mistake
# the checks below exist to prevent for Secrets.
#
# Printed rather than asserted: this script cannot know which version was
# intended. A person reading one line can.
printf 'Images this release will run:\n'
printf '%s\n' "$rendered_chart" | awk '$1 == "image:" { gsub(/"/, "", $2); print "  " $2 }' | sort -u

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

# --force-conflicts because this chart is the source of truth for what it declares.
#
# Helm 4 applies server-side, so a field claimed by another manager makes the
# whole upgrade fail rather than the one field. That other manager is usually a
# person: scaling a Deployment in k9s claims .spec.replicas through the scale
# subresource, and from then on every deploy fails with a conflict until somebody
# works out what a field manager is. Editing an image tag by hand claims that too.
#
# Forcing is the right answer for exactly the fields this chart declares: it takes
# them back, which is what "the chart says how many replicas there are" means. It
# does not touch fields the chart does not declare.
#
# What it WOULD fight is a controller that legitimately owns one of these -- an
# HPA owning replicas is the obvious one. There is none here, and adding one would
# mean removing replicas from this chart rather than dropping this flag.
#
# --server-side=true is stated rather than left to the default, and the default is
# the reason. It is "auto", which means "whatever the previous revision of this
# release used" -- so a release that was last applied client-side keeps being
# applied client-side, and --force-conflicts above silently does nothing, because
# there are no field managers to force against. Both of this lab's releases are
# already on server-side apply, so this changes no behaviour today; what it does is
# stop the flag above from depending on a property of the last install rather than
# on this file.
helm upgrade --install "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file" \
  --server-side=true \
  --force-conflicts \
  --rollback-on-failure \
  --wait \
  --timeout 5m

printf 'Admin release installed.\n'
