#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'Usage: %s --values /absolute/or/relative/values.local.yaml\n' "$0" >&2
}

values_file=''
while (($#)); do
  case "$1" in
    --values)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      values_file=$2
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

[[ -n $values_file && -f $values_file ]] || { usage; exit 2; }
: "${KUBECONFIG:?Set KUBECONFIG to the ignored administrator kubeconfig}"

for command_name in awk grep helm kubectl; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  }
done

if grep -Eq 'example\.(com|invalid)|192\.0\.2\.1' "$values_file"; then
  printf 'Replace every example domain, email, and NFS address in %s\n' "$values_file" >&2
  exit 1
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
chart_dir="$repo_root/charts/platform"
namespace='platform-system'
release='home-lab-platform'
gateway_api_url='https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml'

kubectl apply --server-side --field-manager=home-lab-platform \
  --filename "$gateway_api_url"
kubectl wait --for=condition=Established \
  customresourcedefinition/gateways.gateway.networking.k8s.io \
  customresourcedefinition/httproutes.gateway.networking.k8s.io \
  --timeout=2m

kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "$namespace" \
  home-lab.example/gateway-access=true \
  --overwrite

helm repo add traefik https://traefik.github.io/charts --force-update
helm repo add nfs-subdir-external-provisioner \
  https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner/ \
  --force-update
helm repo add metrics-server \
  https://kubernetes-sigs.github.io/metrics-server/ \
  --force-update
helm dependency build "$chart_dir"

rendered_chart=$(helm template "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file" \
  --skip-crds)

issuer_secret_spec=$(printf '%s\n' "$rendered_chart" | awk '
  $0 == "# Source: home-lab-platform/templates/issuers.yaml" { in_template = 1; next }
  in_template && $1 == "apiTokenSecretRef:" { in_ref = 1; next }
  in_ref && $1 == "name:" { name = $2; next }
  in_ref && $1 == "key:" { print name ":" $2; exit }
')
cloudflared_secret_spec=$(printf '%s\n' "$rendered_chart" | awk '
  $0 == "# Source: home-lab-platform/templates/cloudflared.yaml" { in_template = 1; next }
  in_template && $1 == "-" && $2 == "name:" && $3 == "TUNNEL_TOKEN" { in_token = 1; next }
  in_token && $1 == "name:" { name = $2; next }
  in_token && $1 == "key:" { print name ":" $2; exit }
')

# Same shape as cloudflared's above. Yields nothing when platform.redis.enabled is
# false, and the loop skips an empty spec — so a cluster not running Redis is not
# asked for a Secret it has no use for.
redis_secret_spec=$(printf '%s\n' "$rendered_chart" | awk '
  $0 == "# Source: home-lab-platform/templates/redis.yaml" { in_template = 1; next }
  in_template && $1 == "-" && $2 == "name:" && $3 == "REDIS_PASSWORD" { in_token = 1; next }
  in_token && $1 == "name:" { name = $2; next }
  in_token && $1 == "key:" { print name ":" $2; exit }
')

for secret_spec in "$issuer_secret_spec" "$cloudflared_secret_spec" "$redis_secret_spec"; do
  [[ -n $secret_spec ]] || continue
  secret_name=${secret_spec%%:*}
  secret_key=${secret_spec#*:}
  if [[ -z $(kubectl get secret "$secret_name" --namespace "$namespace" \
    -o "go-template={{ index .data \"$secret_key\" }}" 2>/dev/null) ]]; then
    printf 'Secret %s/%s must exist with key %s\n' \
      "$namespace" "$secret_name" "$secret_key" >&2
    exit 1
  fi
done

helm upgrade --install "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file" \
  --set platform.issuers.enabled=false \
  --set platform.whoami.enabled=false \
  --skip-crds \
  --rollback-on-failure \
  --wait \
  --timeout 10m

kubectl rollout status deployment/cert-manager \
  --namespace "$namespace" --timeout=5m
kubectl rollout status deployment/cert-manager-webhook \
  --namespace "$namespace" --timeout=5m

helm upgrade "$release" "$chart_dir" \
  --namespace "$namespace" \
  --values "$values_file" \
  --skip-crds \
  --rollback-on-failure \
  --wait \
  --timeout 10m

printf 'Platform release installed. Configure the Cloudflare public hostname, then run scripts/validate-platform.sh.\n'
