#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary_directory="$(mktemp -d)"

cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT

cat >"${temporary_directory}/databases.env" <<ENV
DATABASE_DATA_DIR=${temporary_directory}/data
POSTGRES_ADMIN_USER=ci_postgres_admin
POSTGRES_ADMIN_PASSWORD=ci-placeholder-not-a-real-secret
MONGO_ADMIN_USER=ci_mongo_admin
MONGO_ADMIN_PASSWORD=ci-placeholder-not-a-real-secret
ENV

render_compose() {
  local compose_file="$1"

  docker compose \
    --env-file "${temporary_directory}/databases.env" \
    --file "${repo_root}/${compose_file}" \
    config
}

postgres_config="$(render_compose databases/postgres.compose.yaml)"
mongo_config="$(render_compose databases/mongo.compose.yaml)"

grep -q 'image: pgvector/pgvector:0.8.6-pg18-bookworm' <<<"$postgres_config"
grep -q 'image: mongo:7.0.40-jammy' <<<"$mongo_config"
grep -q 'restart: unless-stopped' <<<"$postgres_config"
grep -q 'restart: unless-stopped' <<<"$mongo_config"
grep -q "source: ${temporary_directory}/data/postgres" <<<"$postgres_config"
grep -q "source: ${temporary_directory}/data/mongo" <<<"$mongo_config"
grep -q 'published: "5432"' <<<"$postgres_config"
grep -q 'published: "27017"' <<<"$mongo_config"

printf 'Database Compose validation passed.\n'
