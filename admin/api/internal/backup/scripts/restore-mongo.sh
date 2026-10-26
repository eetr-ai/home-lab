#!/bin/sh
# Restores one MongoDB backup into a database.
#
# Runs in the pinned mongo image, after the download container has put the
# archive in WORK. Arguments: the mode (new or replace), the database the backup
# came from, the database to restore into, and the archive's path.
# ADMIN_MONGO_URI is the panel's connection string.
#
#   new      the target must hold no collections
#   replace  the target is dropped first, so nothing created since the backup
#            is left behind among the restored data
set -eu

mode=$1
source_db=$2
target=$3
archive=$4

# uri_without_database is dump-mongo.sh's, for the same reason: --nsTo names the
# database, and one in the URI's path would disagree with it.
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

# The names are allowlisted by the API, so they are safe in a JavaScript string.
collections=$(mongosh "$uri" --quiet --eval "db.getSiblingDB('$target').getCollectionNames().length")
if [ "$mode" = new ] && [ "$collections" != 0 ]; then
  echo "database $target already holds collections; restore into a new name, or replace it" >&2
  exit 1
fi
if [ "$mode" = replace ]; then
  echo "Dropping $target..."
  mongosh "$uri" --quiet --eval "db.getSiblingDB('$target').dropDatabase()" >/dev/null
fi

echo "Restoring $source_db into $target..."
mongorestore --quiet --uri="$uri" --archive="$archive" --gzip \
  --nsInclude="$source_db.*" --nsFrom="$source_db.*" --nsTo="$target.*"
echo "Restored $source_db into $target."
