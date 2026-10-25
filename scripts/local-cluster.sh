#!/usr/bin/env bash

# Run the admin panel on a throwaway k3d cluster on this machine.
#
#   local-cluster.sh deploy   create or start the cluster and the databases,
#                             build and import the images, install the chart,
#                             and forward the panel to http://localhost:3000
#   local-cluster.sh stop     stop all of it, keeping the data
#   local-cluster.sh delete   remove all of it, data included
#
# The shape is the lab's: PostgreSQL and MongoDB run as plain containers beside
# the cluster (local/compose.yaml), never as pods in it, and the chart is the same
# chart with local/values.yaml on top. What differs is only what a laptop lacks —
# no Gateway in front, no Redis, no agent, no console.
#
# Identity comes from admin/.env, the file `task dev` already uses: the same
# eetr-auth client, whose redirect URIs include http://localhost:3000. Google
# Drive is optional and comes from the same file when GOOGLE_CLIENT_ID is set.
#
# Nothing personal is written into the repository. The generated database
# passwords and the kubeconfig live in a state directory outside it, one per
# machine rather than per checkout, because the cluster and the database volumes
# are per machine too: a second checkout or worktree generating passwords of its
# own would meet volumes initialised with the first one's, and fail to sign in.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cluster='home-lab-local'
network="k3d-${cluster}"
# Pinned, as everything here is. The newest k3s k3d ships as its default; the lab
# runs a newer minor, and nothing the panel uses differs between them.
k3s_image='rancher/k3s:v1.35.5-k3s1'
namespace='admin'
release='home-lab-admin'
databases_namespace='local-databases'
state_dir="${HOME_LAB_LOCAL_STATE:-${XDG_STATE_HOME:-${HOME}/.local/state}/home-lab/local-cluster}"
compose=(docker compose --project-directory "${repo_root}/local" -f "${repo_root}/local/compose.yaml"
  --env-file "${state_dir}/databases.env")
panel_port=3000

usage() {
  printf 'Usage: %s deploy|stop|delete\n' "$0" >&2
}

require() {
  for command_name in "$@"; do
    command -v "$command_name" >/dev/null 2>&1 || {
      printf 'Required command not found: %s\n' "$command_name" >&2
      exit 1
    }
  done
}

say() {
  printf '\n==> %s\n' "$1"
}

cluster_exists() {
  k3d cluster list --no-headers 2>/dev/null | awk '{ print $1 }' | grep -qx "$cluster"
}

# --- deploy --------------------------------------------------------------------

load_env() {
  local env_file="${repo_root}/admin/.env"
  [[ -s $env_file ]] || {
    printf 'admin/.env is missing. Create it from admin/.env.example; the local cluster\n' >&2
    printf 'signs in with the same eetr-auth client that task dev uses.\n' >&2
    exit 1
  }
  set -a
  # shellcheck disable=SC1090 # the operator's own, ignored file
  . "$env_file"
  set +a
  for variable in OIDC_ISSUER OIDC_CLIENT_ID OIDC_CLIENT_SECRET AUTH_SECRET; do
    [[ -n ${!variable:-} ]] || {
      printf '%s must be set in admin/.env\n' "$variable" >&2
      exit 1
    }
  done
}

# The databases' passwords are made once and kept, so the data volumes and the
# Secrets keep agreeing across deploys.
ensure_state() {
  # A data volume with no passwords to match it is one this script can no longer
  # open, and generating new ones would only make that failure confusing.
  if [[ ! -s "${state_dir}/databases.env" ]] && docker volume ls -q |
    grep -qx 'home-lab-local-databases_postgres'; then
    printf 'The local database volumes exist but their passwords in %s do not.\n' "$state_dir" >&2
    printf 'Run task cluster:delete to start over.\n' >&2
    exit 1
  fi
  mkdir -p "$state_dir"
  chmod 700 "$state_dir"
  if [[ ! -s "${state_dir}/databases.env" ]]; then
    umask 077
    printf 'LOCAL_POSTGRES_PASSWORD=%s\nLOCAL_MONGO_PASSWORD=%s\n' \
      "$(openssl rand -hex 24)" "$(openssl rand -hex 24)" >"${state_dir}/databases.env"
  fi
  set -a
  # shellcheck disable=SC1091 # generated just above
  . "${state_dir}/databases.env"
  set +a
}

ensure_cluster() {
  if cluster_exists; then
    say "Starting the k3d cluster ${cluster}..."
    k3d cluster start "$cluster" --wait >/dev/null
  else
    say "Creating the k3d cluster ${cluster}..."
    # No load balancer and no Traefik: the panel is reached by port-forward, the
    # way the chart's authUrl comment describes, not through an ingress.
    k3d cluster create "$cluster" \
      --image "$k3s_image" \
      --no-lb \
      --k3s-arg '--disable=traefik@server:0' \
      --kubeconfig-update-default=false \
      --wait >/dev/null
  fi
  # A kubeconfig of its own, so deploying never touches ~/.kube/config or changes
  # which cluster kubectl talks to in another terminal.
  k3d kubeconfig get "$cluster" >"${state_dir}/kubeconfig"
  chmod 600 "${state_dir}/kubeconfig"
  export KUBECONFIG="${state_dir}/kubeconfig"
  # k3s installs CoreDNS as an add-on a few seconds after the API answers, so
  # `--wait` returns before the Deployment even exists, and a rollout check on
  # something absent fails rather than waits. Wait for it to appear first.
  local attempts=0
  until kubectl -n kube-system get deployment coredns >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if ((attempts > 60)); then
      printf 'CoreDNS never appeared in the cluster\n' >&2
      exit 1
    fi
    sleep 2
  done
  kubectl -n kube-system rollout status deployment/coredns --timeout=120s >/dev/null
}

# Pods cannot resolve a Docker container's name — the cluster's DNS knows only
# Services — so each database is published as a selectorless Service whose
# EndpointSlice holds the container's address on the cluster network. That
# address is read at deploy time and never written anywhere else.
publish_database() {
  local name=$1 port=$2 container address
  container=$("${compose[@]}" ps -q "$name")
  address=$(docker inspect -f "{{ (index .NetworkSettings.Networks \"${network}\").IPAddress }}" "$container")
  [[ -n $address ]] || {
    printf 'The %s container has no address on %s\n' "$name" "$network" >&2
    exit 1
  }

  kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Service
metadata:
  name: ${name}
  namespace: ${databases_namespace}
spec:
  ports:
    - name: ${name}
      port: ${port}
---
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: ${name}
  namespace: ${databases_namespace}
  labels:
    kubernetes.io/service-name: ${name}
addressType: IPv4
ports:
  - name: ${name}
    port: ${port}
endpoints:
  - addresses: ["${address}"]
YAML
}

ensure_databases() {
  say "Starting PostgreSQL and MongoDB beside the cluster..."
  "${compose[@]}" up --detach --wait --quiet-pull >/dev/null 2>&1 || {
    printf 'The databases did not become healthy:\n' >&2
    "${compose[@]}" ps >&2
    exit 1
  }

  kubectl create namespace "$databases_namespace" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  publish_database postgres 5432
  publish_database mongo 27017

  # The panel's own database, for declared Helm deployments, on the same server —
  # as in the lab, where it is a separate database beside the ones it manages.
  if ! "${compose[@]}" exec -T postgres psql -U postgres -tAc \
    "SELECT 1 FROM pg_database WHERE datname = 'admin_panel'" | grep -q 1; then
    "${compose[@]}" exec -T postgres createdb -U postgres admin_panel
  fi
}

# Every image a pod here runs. The panel's are built from this checkout under a
# fresh tag, because the chart refuses latest and a reused tag would let the
# cluster keep running whatever it imported last time. The dump images are the
# ones the backup jobs use, imported because the chart's pullPolicy is Never.
build_and_import() {
  local tag=$1
  say "Building the panel images (${tag})..."
  docker build --quiet -t "home-lab/admin-api:${tag}" "${repo_root}/admin/api" >/dev/null
  docker build --quiet -t "home-lab/admin-web:${tag}" "${repo_root}/admin/web" >/dev/null

  local dump_images
  dump_images=$(awk '/^ +images:$/ { on = 1; next } on && /^ +(postgres|mongo):/ { print $2 } on && !/^ +(postgres|mongo):/ { on = 0 }' \
    "${repo_root}/charts/admin/values.yaml")
  for image in $dump_images; do
    docker image inspect "$image" >/dev/null 2>&1 || docker pull --quiet "$image" >/dev/null
  done

  say "Importing the images into the cluster..."
  # Through one archive saved for this machine's platform only. Docker's
  # containerd image store keeps a pulled image as its whole multi-platform
  # index with only one platform's layers present, and a plain import walks the
  # index into layers that were never downloaded and fails on a missing digest.
  local platform archive="${state_dir}/images.tar"
  platform="linux/$(docker version --format '{{.Server.Arch}}')"
  # shellcheck disable=SC2086 # one word per image, on purpose
  docker save --platform "$platform" -o "$archive" \
    "home-lab/admin-api:${tag}" "home-lab/admin-web:${tag}" $dump_images
  k3d image import --cluster "$cluster" "$archive" >/dev/null
  rm -f "$archive"
}

apply_secret() {
  local name=$1
  shift
  kubectl -n "$namespace" create secret generic "$name" "$@" --dry-run=client -o yaml |
    kubectl apply -f - >/dev/null
}

install_panel() {
  local tag=$1 gateway_api_url
  # The same pinned Gateway API the platform installer applies, read out of it so
  # the two cannot drift. The chart renders an HTTPRoute, so the CRDs must exist
  # even though nothing here serves it.
  gateway_api_url=$(sed -n "s/^gateway_api_url='\(.*\)'$/\1/p" "${repo_root}/scripts/install-platform.sh")
  kubectl apply --server-side -f "$gateway_api_url" >/dev/null

  kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

  local postgres_host="postgres.${databases_namespace}.svc.cluster.local:5432"
  apply_secret admin-oidc \
    --from-literal=authSecret="$AUTH_SECRET" --from-literal=clientSecret="$OIDC_CLIENT_SECRET"
  apply_secret admin-postgres \
    --from-literal=dsn="postgres://postgres:${LOCAL_POSTGRES_PASSWORD}@${postgres_host}/postgres?sslmode=disable"
  apply_secret admin-helm-postgres \
    --from-literal=dsn="postgres://postgres:${LOCAL_POSTGRES_PASSWORD}@${postgres_host}/admin_panel?sslmode=disable"
  apply_secret admin-mongo \
    --from-literal=uri="mongodb://root:${LOCAL_MONGO_PASSWORD}@mongo.${databases_namespace}.svc.cluster.local:27017/?authSource=admin"
  if [[ -n ${GOOGLE_CLIENT_ID:-} && -n ${GOOGLE_CLIENT_SECRET:-} ]]; then
    apply_secret admin-google-drive \
      --from-literal=clientId="$GOOGLE_CLIENT_ID" --from-literal=clientSecret="$GOOGLE_CLIENT_SECRET"
  fi

  say "Installing the panel..."
  local settings=(
    --set "admin.image.tag=${tag}"
    --set "admin.api.oidc.issuer=${OIDC_ISSUER}"
    --set "admin.web.clientId=${OIDC_CLIENT_ID}"
  )
  [[ -z ${OIDC_AUDIENCE:-} ]] || settings+=(--set "admin.api.oidc.audience=${OIDC_AUDIENCE}")
  [[ -z ${GOOGLE_PICKER_API_KEY:-} ]] ||
    settings+=(--set "admin.api.backups.google.picker.apiKey=${GOOGLE_PICKER_API_KEY}")
  [[ -z ${GOOGLE_PICKER_APP_ID:-} ]] ||
    settings+=(--set-string "admin.api.backups.google.picker.appId=${GOOGLE_PICKER_APP_ID}")

  helm upgrade --install "$release" "${repo_root}/charts/admin" \
    --namespace "$namespace" \
    --values "${repo_root}/local/values.yaml" \
    "${settings[@]}" \
    --wait --timeout 5m >/dev/null
}

stop_forward() {
  local pid_file="${state_dir}/port-forward.pid"
  if [[ -s $pid_file ]] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
    kill "$(cat "$pid_file")"
  fi
  rm -f "$pid_file"
}

# In the background, so the task returns. Its log is beside its pid, and
# `task cluster:stop` is what ends it.
start_forward() {
  stop_forward
  nohup kubectl -n "$namespace" port-forward service/admin-web "${panel_port}:80" \
    >"${state_dir}/port-forward.log" 2>&1 &
  printf '%s\n' "$!" >"${state_dir}/port-forward.pid"
  sleep 2
  kill -0 "$(cat "${state_dir}/port-forward.pid")" 2>/dev/null || {
    printf 'The port-forward did not start; is something already on port %s?\n' "$panel_port" >&2
    cat "${state_dir}/port-forward.log" >&2
    exit 1
  }
}

deploy() {
  require docker k3d kubectl helm openssl awk sed
  load_env
  ensure_state
  ensure_cluster
  ensure_databases

  local tag
  tag="local-$(date -u +%Y%m%d%H%M%S)"
  build_and_import "$tag"
  install_panel "$tag"
  start_forward

  printf '\nThe panel is at http://localhost:%s\n' "$panel_port"
  printf 'kubectl for this cluster: export KUBECONFIG=%s\n' "${state_dir}/kubeconfig"
  printf 'Stop it with: task cluster:stop. Remove it, data included, with: task cluster:delete.\n'
}

# --- stop and delete -------------------------------------------------------------

stop() {
  require docker k3d
  stop_forward
  if cluster_exists; then
    say "Stopping the k3d cluster ${cluster}..."
    k3d cluster stop "$cluster" >/dev/null
  fi
  if [[ -s "${state_dir}/databases.env" ]]; then
    say "Stopping the databases..."
    "${compose[@]}" stop >/dev/null
  fi
  printf '\nStopped. The data is kept, and task cluster:deploy brings it all back.\n'
}

delete() {
  require docker k3d
  stop_forward
  if [[ -s "${state_dir}/databases.env" ]]; then
    say "Removing the databases and their data..."
    "${compose[@]}" down --volumes >/dev/null
  fi
  if cluster_exists; then
    say "Deleting the k3d cluster ${cluster}..."
    k3d cluster delete "$cluster" >/dev/null
  fi
  rm -rf "$state_dir"
  printf '\nDeleted.\n'
}

case "${1:-}" in
  deploy) deploy ;;
  stop) stop ;;
  delete) delete ;;
  *)
    usage
    exit 2
    ;;
esac
