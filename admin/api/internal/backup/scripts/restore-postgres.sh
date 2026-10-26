#!/bin/sh
# Restores one PostgreSQL backup into a database.
#
# Runs in the pinned postgres image, after the download container has put the
# archive in WORK. Arguments: the mode (new or replace), the database the backup
# came from, the database to restore into, and the archive's path.
# ADMIN_POSTGRES_DSN is the panel's superuser connection string.
#
#   new      the target must not exist; it is created with the source's owner
#   replace  the target is dropped (its sessions ended) and created again with
#            the owner it had, then restored
#
# Roles: the archive names the roles that own its objects and are granted on
# them. Any the server lacks are created NOLOGIN with no password, and no
# existing role is touched — replaying the night's globals would reset the
# passwords of roles that exist today. The names are passed to psql as
# variables and quoted by format(%I), never pasted into SQL.
set -eu

mode=$1
source_db=$2
target=$3
archive=$4

# dsn_for points the connection string at one database; see dump-postgres.sh,
# which has the same function and the reasoning for both spellings.
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

maintenance=$(dsn_for postgres)

# owner_of prints a database's owner, or nothing when it does not exist.
owner_of() {
  psql --dbname="$maintenance" -v ON_ERROR_STOP=1 -v name="$1" -tA <<'SQL'
SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = :'name';
SQL
}

# Read the archive whole before anything is created or dropped. The role
# extraction below reads it through a pipeline, whose failure set -e does not
# see — so without this an unreadable archive would go on to drop the live
# database in replace mode and leave nothing in its place.
echo "Checking the archive..."
pg_restore --list "$archive" >/dev/null

target_owner=$(owner_of "$target")
if [ "$mode" = new ] && [ -n "$target_owner" ]; then
  echo "database $target already exists; restore into a new name, or replace it" >&2
  exit 1
fi

echo "Creating the roles the backup needs that this server lacks..."
pg_restore --schema-only --file=- "$archive" |
  sed -nE 's/.*OWNER TO ([^;]+);.*/\1/p; s/^GRANT .* TO ([^;]+);.*/\1/p' |
  sed -E 's/ (WITH GRANT OPTION|GRANTED BY .*)$//' |
  tr ',' '\n' | sed -E 's/^ +//; s/ +$//; s/^"(.*)"$/\1/' |
  grep -vx 'PUBLIC' | sort -u |
  while IFS= read -r role; do
    [ -n "$role" ] || continue
    psql --dbname="$maintenance" -v ON_ERROR_STOP=1 -v role="$role" -q <<'SQL'
SELECT format('CREATE ROLE %I NOLOGIN', :'role')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'role') \gexec
SQL
  done

if [ "$mode" = replace ]; then
  owner=${target_owner:-postgres}
  if [ -n "$target_owner" ]; then
    echo "Dropping $target, ending its sessions..."
    dropdb --maintenance-db="$maintenance" --force "$target"
  fi
else
  # A new database takes the source's owner when the source is still here, so
  # the copy can be swapped in without re-granting anything.
  owner=$(owner_of "$source_db")
  owner=${owner:-postgres}
fi

echo "Creating $target, owned by $owner..."
createdb --maintenance-db="$maintenance" --owner="$owner" "$target"

echo "Restoring $source_db into $target..."
pg_restore --dbname="$(dsn_for "$target")" "$archive"
echo "Restored $source_db into $target."
