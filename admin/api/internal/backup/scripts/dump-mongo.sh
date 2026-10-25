#!/bin/sh
# Dumps MongoDB databases into the backup pod's scratch volume.
#
# Runs in the pinned mongo image, which ships the database tools, as an init
# container. The arguments are the database names; ADMIN_MONGO_URI is the panel's
# connection string and WORK is the scratch directory. The naming is a contract
# with backup-upload:
#
#   mongo__<database>.archive.gz     a mongodump --archive --gzip
#   mongo__<database>.failed         the dump failed; the file holds its stderr
#
# Like the PostgreSQL script, a failure is recorded and the loop carries on.
set -u
umask 077

# uri_without_database drops the database from the URI's path, because mongodump
# refuses a --db that disagrees with one there.
#
# That path is also the default authSource when the URI names none. Dropping it
# would quietly move authentication to `admin`, so when there is no authSource it
# is carried over as one.
uri_without_database() {
  query=""
  case "$ADMIN_MONGO_URI" in
    *\?*) query="${ADMIN_MONGO_URI#*\?}" ;;
  esac
  base=${ADMIN_MONGO_URI%%\?*}
  scheme=${base%%://*}
  rest=${base#*://}
  authority=${rest%%/*}
  path=""
  case "$rest" in
    */*) path=${rest#*/} ;;
  esac

  if [ -n "$path" ]; then
    case "&$query" in
      *"&authSource="*) ;;
      *) query="${query:+$query&}authSource=$path" ;;
    esac
  fi
  printf '%s://%s/%s' "$scheme" "$authority" "${query:+?$query}"
}

uri=$(uri_without_database)

for database in "$@"; do
  echo "Dumping MongoDB database $database..."
  if mongodump --quiet --uri="$uri" --db="$database" --gzip \
    --archive="$WORK/mongo__$database.archive.gz" 2>"$WORK/mongo__$database.err"; then
    rm -f "$WORK/mongo__$database.err"
  else
    rm -f "$WORK/mongo__$database.archive.gz"
    mv "$WORK/mongo__$database.err" "$WORK/mongo__$database.failed"
    printf 'the dump of %s failed:\n' "$database" >&2
    cat "$WORK/mongo__$database.failed" >&2
  fi
done
