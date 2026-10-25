#!/usr/bin/env bash

# Test the backup pod's dump scripts against fake pg_dump, pg_dumpall and
# mongodump.
#
# The scripts run in the postgres and mongo images, so they are exercised here
# with stand-ins that record the arguments they were given and fail for a
# database named `broken`. What is asserted is the contract with backup-upload:
# which files are left in WORK, and what each tool was pointed at. No database is
# involved.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scripts="${repo_root}/admin/api/internal/backup/scripts"

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

fakes="${scratch}/bin"
mkdir -p "$fakes"

# Every fake appends its arguments to calls.log, one call per line, and fails
# when any argument names the database `broken`. It writes its output file
# before failing, the way a real dump that dies part-way leaves a truncated one
# behind — which is what the scripts have to clean up.
for tool in pg_dump pg_dumpall mongodump; do
  cat >"${fakes}/${tool}" <<'FAKE'
#!/bin/sh
printf '%s %s\n' "$(basename "$0")" "$*" >>"$CALLS"
for argument in "$@"; do
  case "$argument" in
    --file=*) echo dump >"${argument#--file=}" ;;
    --archive=*) echo dump >"${argument#--archive=}" ;;
  esac
done
for argument in "$@"; do
  case "$argument" in
    *broken*) echo "fake: database broken does not exist" >&2; exit 1 ;;
  esac
done
case "$(basename "$0")" in
  pg_dumpall) echo "CREATE ROLE app;" ;;
esac
FAKE
  chmod +x "${fakes}/${tool}"
done

failures=0
fail() {
  printf 'FAIL %s\n' "$1" >&2
  failures=$((failures + 1))
}
pass() { printf 'ok   %s\n' "$1"; }

# run_script <script> <connection-variable> <connection-value> <databases...>
run_script() {
  local script=$1 variable=$2 value=$3
  shift 3
  work="${scratch}/work-$RANDOM"
  mkdir -p "$work"
  export CALLS="${work}.calls"
  : >"$CALLS"
  env PATH="${fakes}:$PATH" WORK="$work" "$variable=$value" \
    sh "${scripts}/${script}" "$@" >/dev/null 2>"${work}.stderr"
}

expect_file() {
  if [[ -f "${work}/$1" ]]; then pass "$2"; else fail "$2 (no ${work##*/}/$1)"; fi
}
expect_no_file() {
  if [[ ! -e "${work}/$1" ]]; then pass "$2"; else fail "$2 (${work##*/}/$1 exists)"; fi
}
expect_call() {
  if grep -qF -- "$1" "$CALLS"; then pass "$2"; else fail "$2 (calls: $(tr '\n' '|' <"$CALLS"))"; fi
}

# --- PostgreSQL ---------------------------------------------------------------

if run_script dump-postgres.sh ADMIN_POSTGRES_DSN \
  'postgres://admin:pw@db.example.invalid:5432/postgres?sslmode=disable' app broken; then
  pass "dump-postgres.sh exits zero when one database fails"
else
  fail "dump-postgres.sh exits zero when one database fails"
fi
expect_file 'postgres__app.dump' "a good PostgreSQL database leaves its archive"
expect_file 'postgres__@globals.sql.gz' "the globals are dumped and compressed"
expect_file 'postgres__broken.failed' "a failed database leaves a .failed marker"
expect_no_file 'postgres__broken.dump' "a failed database leaves no partial archive"
expect_no_file 'postgres__app.err' "a good database leaves no stderr file behind"
if grep -q 'does not exist' "${work}/postgres__broken.failed"; then
  pass "the marker carries the tool's own error"
else
  fail "the marker carries the tool's own error"
fi
expect_call 'pg_dump --format=custom --dbname=postgres://admin:pw@db.example.invalid:5432/app?sslmode=disable' \
  "a URI DSN has its database swapped and its query kept"
expect_call 'pg_dumpall --globals-only --dbname=postgres://admin:pw@db.example.invalid:5432/postgres?sslmode=disable' \
  "the globals are read through the DSN as given"

run_script dump-postgres.sh ADMIN_POSTGRES_DSN 'postgresql://admin:pw@db.example.invalid' app
expect_call '--dbname=postgresql://admin:pw@db.example.invalid/app ' "a URI with no database gains one"

run_script dump-postgres.sh ADMIN_POSTGRES_DSN 'host=db.example.invalid user=admin dbname=postgres' app
expect_call "--dbname=host=db.example.invalid user=admin dbname=postgres dbname='app'" \
  "a key/value DSN has dbname appended, which libpq reads last-wins"

# --- MongoDB ------------------------------------------------------------------

if run_script dump-mongo.sh ADMIN_MONGO_URI 'mongodb://root:pw@db.example.invalid:27017/?authSource=admin' \
  app broken; then
  pass "dump-mongo.sh exits zero when one database fails"
else
  fail "dump-mongo.sh exits zero when one database fails"
fi
expect_file 'mongo__app.archive.gz' "a good MongoDB database leaves its archive"
expect_file 'mongo__broken.failed' "a failed MongoDB database leaves a .failed marker"
expect_no_file 'mongo__broken.archive.gz' "a failed MongoDB database leaves no partial archive"
expect_call '--uri=mongodb://root:pw@db.example.invalid:27017/?authSource=admin --db=app --gzip' \
  "a URI with no database is used as given"

run_script dump-mongo.sh ADMIN_MONGO_URI 'mongodb://root:pw@db.example.invalid:27017/admin' app
expect_call '--uri=mongodb://root:pw@db.example.invalid:27017/?authSource=admin --db=app' \
  "a database in the URI path is dropped and kept as its authSource"

run_script dump-mongo.sh ADMIN_MONGO_URI 'mongodb://root:pw@db.example.invalid/app?authSource=admin&tls=true' app
expect_call '--uri=mongodb://root:pw@db.example.invalid/?authSource=admin&tls=true --db=app' \
  "an explicit authSource wins over the path"

if ((failures > 0)); then
  printf '%d backup dump script checks failed\n' "$failures" >&2
  exit 1
fi
printf 'all backup dump script checks passed\n'
