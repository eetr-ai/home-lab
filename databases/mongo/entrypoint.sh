#!/usr/bin/env bash

set -Eeuo pipefail

readonly source_secret="${MONGO_INITDB_ROOT_PASSWORD_FILE:?MONGO_INITDB_ROOT_PASSWORD_FILE is required}"
readonly runtime_secret_directory="/run/home-lab-secrets"
readonly runtime_secret="${runtime_secret_directory}/mongo_admin_password"

# Local Docker Compose implements file-backed secrets as bind mounts, so a
# mode-0600 host file remains root-only in the container. The official MongoDB
# entrypoint drops privileges before reading its *_FILE input. Copy the secret
# into a private tmpfs with the ownership MongoDB needs, then let the official
# entrypoint retain responsibility for initialization and credential cleanup.
chown mongodb:mongodb "$runtime_secret_directory"
chmod 0700 "$runtime_secret_directory"
install -o mongodb -g mongodb -m 0400 "$source_secret" "$runtime_secret"

export MONGO_INITDB_ROOT_PASSWORD_FILE="$runtime_secret"
exec /usr/local/bin/docker-entrypoint.sh "$@"
