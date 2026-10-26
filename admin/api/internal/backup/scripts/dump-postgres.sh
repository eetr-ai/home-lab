#!/bin/sh
# Dumps PostgreSQL databases into the backup pod's scratch volume.
#
# Runs in the pinned postgres image, as an init container. The arguments are the
# database names; ADMIN_POSTGRES_DSN is the panel's superuser connection string
# and WORK is the scratch directory. The upload container reads what is left
# there by name, so the naming below is a contract with backup-upload:
#
#   postgres__<database>.dump        a pg_dump custom-format archive
#   postgres__@globals.sql.gz        roles and tablespaces, from pg_dumpall
#   postgres__<database>.failed      the dump failed; the file holds its stderr
#
# A failed database does not stop the others, and the script still exits zero:
# one broken database should not cost every other one its backup. The upload
# container reports the failures and fails the Job, so nothing is hidden.
set -u
umask 077

# dsn_for points the connection string at one database.
#
# Both libpq spellings are handled. A URI carries the database as its path, so
# that is replaced and the query string kept. A key/value string takes the last
# value of a repeated keyword, so appending dbname= wins over any already there.
dsn_for() {
  case "$ADMIN_POSTGRES_DSN" in
    postgres://* | postgresql://*)
      query=""
      case "$ADMIN_POSTGRES_DSN" in
        *\?*) query="?${ADMIN_POSTGRES_DSN#*\?}" ;;
      esac
      base=${ADMIN_POSTGRES_DSN%%\?*}
      scheme=${base%%://*}
      rest=${base#*://}
      authority=${rest%%/*}
      printf '%s://%s/%s%s' "$scheme" "$authority" "$1" "$query"
      ;;
    *)
      printf "%s dbname='%s'" "$ADMIN_POSTGRES_DSN" "$1"
      ;;
  esac
}

# record_failure turns a dump's stderr into the marker the uploader reads.
record_failure() {
  mv "$WORK/$1.err" "$WORK/$1.failed"
  printf 'the dump of %s failed:\n' "$1" >&2
  cat "$WORK/$1.failed" >&2
}

echo "Dumping the PostgreSQL globals..."
if pg_dumpall --globals-only --dbname="$ADMIN_POSTGRES_DSN" \
  >"$WORK/postgres__@globals.sql" 2>"$WORK/postgres__@globals.err" &&
  gzip -9 "$WORK/postgres__@globals.sql" 2>>"$WORK/postgres__@globals.err"; then
  rm -f "$WORK/postgres__@globals.err"
else
  rm -f "$WORK/postgres__@globals.sql" "$WORK/postgres__@globals.sql.gz"
  record_failure "postgres__@globals"
fi

for database in "$@"; do
  echo "Dumping PostgreSQL database $database..."
  if pg_dump --format=custom --dbname="$(dsn_for "$database")" \
    --file="$WORK/postgres__$database.dump" 2>"$WORK/postgres__$database.err"; then
    rm -f "$WORK/postgres__$database.err"
  else
    rm -f "$WORK/postgres__$database.dump"
    record_failure "postgres__$database"
  fi
done
