#!/usr/bin/env bash

# Test the restore scripts against fake PostgreSQL and MongoDB tools.
#
# Like backup-dump-scripts.sh, no database is involved: the fakes record what
# they were asked and answer from a small stand-in for the server's state —
# which databases exist, who owns them, which roles there are, how many
# collections a Mongo database holds. What is asserted is the part that decides
# whether live data survives: what is dropped, what is created and by whom, and
# that an existing role is never touched.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scripts="${repo_root}/admin/api/internal/backup/scripts"

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
fakes="${scratch}/bin"
mkdir -p "$fakes"

# psql reads its SQL from stdin. It answers the owner lookup from FAKE_DBS
# (name:owner,...). A CREATE ROLE guarded by WHERE NOT EXISTS is recorded only
# for roles not in FAKE_ROLES, the way the real server would run it; one
# without the guard is recorded for every role, so losing the guard shows.
cat >"${fakes}/psql" <<'FAKE'
#!/bin/sh
name="" role=""
while [ $# -gt 0 ]; do
  case "$1" in
    -v) case "$2" in name=*) name=${2#name=} ;; role=*) role=${2#role=} ;; esac; shift ;;
  esac
  shift
done
sql=$(cat)
case "$sql" in
  *pg_get_userbyid*)
    for pair in $(echo "$FAKE_DBS" | tr ',' ' '); do
      [ "${pair%%:*}" = "$name" ] && echo "${pair#*:}"
    done ;;
  *"CREATE ROLE"*"WHERE NOT EXISTS"*)
    case ",$FAKE_ROLES," in *",$role,"*) ;; *) echo "create-role $role" >>"$CALLS" ;; esac ;;
  # Without the guard the real server would try every role, existing or not.
  *"CREATE ROLE"*) echo "create-role $role" >>"$CALLS" ;;
esac
# As psql does: a query that matched nothing still succeeded.
exit 0
FAKE
for tool in createdb dropdb mongorestore; do
  # shellcheck disable=SC2016 # $* and $CALLS belong to the fake, not to this script
  printf '#!/bin/sh\necho "%s $*" >>"$CALLS"\n' "$tool" >"${fakes}/${tool}"
done
# pg_restore --schema-only prints the archive's DDL; anything else restores.
cat >"${fakes}/pg_restore" <<'FAKE'
#!/bin/sh
case "$*" in
  *--schema-only*)
    echo 'ALTER TABLE public.items OWNER TO app_owner;'
    echo 'ALTER SCHEMA reports OWNER TO "Reporting";'
    echo 'GRANT SELECT ON TABLE public.items TO readonly, PUBLIC;'
    echo 'GRANT ALL ON TABLE public.items TO app_owner;' ;;
  *) echo "pg_restore $*" >>"$CALLS" ;;
esac
FAKE
cat >"${fakes}/mongosh" <<'FAKE'
#!/bin/sh
case "$*" in
  *getCollectionNames*) echo "${FAKE_COLLECTIONS:-0}" ;;
  *dropDatabase*) echo "mongosh drop $*" >>"$CALLS" ;;
esac
FAKE
chmod +x "${fakes}"/*

failures=0
fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }
pass() { printf 'ok   %s\n' "$1"; }

# restore <script> <variable> <value> <mode> <source> <target>; the exit code is
# left in $status.
expect_status() {
  if [[ $status == "$1" ]]; then pass "$2"; else fail "$2 (exit $status)"; fi
}
expect_failed() {
  if [[ $status != 0 ]]; then pass "$1"; else fail "$1 (exit 0)"; fi
}
restore() {
  local script=$1 variable=$2 value=$3
  shift 3
  export CALLS="${scratch}/calls-$RANDOM"
  : >"$CALLS"
  set +e
  env PATH="${fakes}:$PATH" "$variable=$value" sh "${scripts}/${script}" "$@" /work/restore.x \
    >/dev/null 2>"${CALLS}.stderr"
  status=$?
  set -e
}
expect_call() {
  if grep -qF -- "$1" "$CALLS"; then pass "$2"; else fail "$2 (calls: $(tr '\n' '|' <"$CALLS"))"; fi
}
expect_no_call() {
  if grep -qF -- "$1" "$CALLS"; then fail "$2 (calls: $(tr '\n' '|' <"$CALLS"))"; else pass "$2"; fi
}
dsn='postgres://admin:pw@db.example.invalid:5432/postgres?sslmode=disable'

# --- PostgreSQL ---------------------------------------------------------------

FAKE_DBS='app:app_owner' FAKE_ROLES='app_owner,readonly' \
  restore restore-postgres.sh ADMIN_POSTGRES_DSN "$dsn" new app app_copy
expect_status 0 "a new-database restore succeeds"
expect_call 'create-role Reporting' "a missing role is created, its quotes taken off"
expect_no_call 'create-role app_owner' "an existing role is never touched"
expect_no_call 'create-role PUBLIC' "PUBLIC is not a role to create"
expect_call 'createdb --maintenance-db=postgres://admin:pw@db.example.invalid:5432/postgres?sslmode=disable --owner=app_owner app_copy' \
  "the new database takes the source's owner"
expect_no_call 'dropdb' "a new-database restore drops nothing"
expect_call 'pg_restore --dbname=postgres://admin:pw@db.example.invalid:5432/app_copy?sslmode=disable' \
  "it restores into the new database"

FAKE_DBS='app:app_owner,app_copy:someone' FAKE_ROLES='app_owner' \
  restore restore-postgres.sh ADMIN_POSTGRES_DSN "$dsn" new app app_copy
expect_failed "a new database that already exists is refused"
expect_no_call 'createdb' "a refused restore creates nothing"
expect_no_call 'pg_restore --dbname' "a refused restore restores nothing"

FAKE_DBS='app:app_owner' FAKE_ROLES='app_owner,readonly,Reporting' \
  restore restore-postgres.sh ADMIN_POSTGRES_DSN "$dsn" replace app app
expect_call 'dropdb --maintenance-db=postgres://admin:pw@db.example.invalid:5432/postgres?sslmode=disable --force app' \
  "replace drops the database, ending its sessions"
expect_call '--owner=app_owner app' "replace recreates it with the owner it had"

# --- MongoDB ------------------------------------------------------------------

FAKE_COLLECTIONS=0 restore restore-mongo.sh ADMIN_MONGO_URI 'mongodb://root:pw@db.example.invalid/?authSource=admin' \
  new events events_copy
expect_call '--nsInclude=events.* --nsFrom=events.* --nsTo=events_copy.*' "a Mongo restore renames into the target"
expect_no_call 'mongosh drop' "a new-database Mongo restore drops nothing"

FAKE_COLLECTIONS=3 restore restore-mongo.sh ADMIN_MONGO_URI 'mongodb://root:pw@db.example.invalid/?authSource=admin' \
  new events events_copy
expect_failed "a Mongo target that holds collections is refused"
expect_no_call 'mongorestore' "a refused Mongo restore restores nothing"

FAKE_COLLECTIONS=3 restore restore-mongo.sh ADMIN_MONGO_URI 'mongodb://root:pw@db.example.invalid/admin' \
  replace events events
expect_call "mongosh drop" "replace drops the Mongo database first"
expect_call '--uri=mongodb://root:pw@db.example.invalid/?authSource=admin' "the URI's database becomes its authSource"

if ((failures > 0)); then
  printf '%d backup restore script checks failed\n' "$failures" >&2
  exit 1
fi
printf 'all backup restore script checks passed\n'
