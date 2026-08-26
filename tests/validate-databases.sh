#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary_directory="$(mktemp -d)"
readonly placeholder_secret="ci-placeholder-not-a-real-secret"

cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT

printf '%s\n' "$placeholder_secret" >"${temporary_directory}/postgres_admin_password"
printf '%s\n' "$placeholder_secret" >"${temporary_directory}/mongo_admin_password"
chmod 0600 \
  "${temporary_directory}/postgres_admin_password" \
  "${temporary_directory}/mongo_admin_password"

cat >"${temporary_directory}/databases.env" <<EOF
DATABASE_LAN_ADDRESS=192.0.2.10
DATABASE_BRIDGE_INTERFACE=br0
DATABASE_ALLOWED_CLIENTS=192.0.2.11,192.0.2.12,192.0.2.13
DATABASE_SECRETS_DIR=${temporary_directory}
POSTGRES_ADMIN_USER=ci_postgres_admin
POSTGRES_DATABASE=ci_database
MONGO_ADMIN_USER=ci_mongo_admin
EOF

render_compose() {
  local compose_file="$1"

  docker compose \
    --env-file "${temporary_directory}/databases.env" \
    --file "${repo_root}/${compose_file}" \
    config
}

postgres_config="$(render_compose databases/postgres.compose.yaml)"
mongo_config="$(render_compose databases/mongo.compose.yaml)"
combined_config="${postgres_config}${mongo_config}"

grep -q 'image: pgvector/pgvector:0.8.6-pg18-bookworm' <<<"$postgres_config"
grep -q 'image: mongo:8.0.29-noble' <<<"$mongo_config"
grep -q 'POSTGRES_PASSWORD_FILE: /run/secrets/postgres_admin_password' <<<"$postgres_config"
grep -q 'MONGO_INITDB_ROOT_PASSWORD_FILE: /run/secrets/mongo_admin_password' <<<"$mongo_config"
grep -q 'target: /usr/local/bin/home-lab-mongo-entrypoint.sh' <<<"$mongo_config"
grep -q -- '- /run/home-lab-secrets:rw,noexec,nosuid,nodev,size=1m' <<<"$mongo_config"

if grep -qF "$placeholder_secret" <<<"$combined_config"; then
  printf 'Rendered Compose configuration exposed secret contents\n' >&2
  exit 1
fi

if grep -Eq '^[[:space:]]+(POSTGRES_PASSWORD|MONGO_INITDB_ROOT_PASSWORD):' \
  <<<"$combined_config"; then
  printf 'Database passwords must be supplied through *_FILE inputs\n' >&2
  exit 1
fi

if grep -Eq 'host_ip: (0\.0\.0\.0|::)' <<<"$combined_config"; then
  printf 'Database ports must never use a wildcard host binding\n' >&2
  exit 1
fi

for port in 5432 27017; do
  binding_count="$(grep -A4 'host_ip:' <<<"$combined_config" | grep -c "target: ${port}" || true)"
  if [[ "$binding_count" != "2" ]]; then
    printf 'Expected exactly two host bindings for port %s, found %s\n' \
      "$port" "$binding_count" >&2
    exit 1
  fi
done

grep -q '^CREATE EXTENSION IF NOT EXISTS vector;$' \
  "${repo_root}/databases/postgres/initdb/001-extensions.sql"
grep -q '^CREATE EXTENSION IF NOT EXISTS pgcrypto;$' \
  "${repo_root}/databases/postgres/initdb/001-extensions.sql"

printf 'Database Compose validation passed.\n'
