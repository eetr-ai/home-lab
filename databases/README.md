# Host databases

PostgreSQL (with pgvector) and MongoDB run as two independent Docker Compose
projects directly on the virtualization host `eetr01`. They do not run in a VM
or in Kubernetes.

Compose is the whole manager here:

- `restart: unless-stopped` keeps both containers up across crashes and reboots.
- Data lives in bind-mounted directories under `DATABASE_DATA_DIR`, so it is
  obvious on disk and easy to back up.
- Ports 5432 and 27017 are published on the host, so the laptop, the host
  itself, and the Kubernetes nodes all connect the same way.

The host is `eetr01` at `10.0.0.240` on `br0`, so that is the address every
client uses. Access control is the database password: there is no per-client
allowlist and no Cloudflare route — never forward these ports from the router
to the internet.

Each server gets exactly one account, and it is a superuser. That account is
what the infra admin application — the in-cluster tool that will manage these
databases and the cluster itself — connects with, and it is what `psql` and
`mongosh` use from the laptop. Creating databases, roles, collections, or
narrower users is that application's job, not this module's.

## Set up

The repository stays on your laptop. Docker Compose talks to `eetr01` over SSH
through a Docker context, so nothing is copied to the server — the containers,
the published ports, and the data directories are all created there while you
drive from your checkout.

Create the context once:

```bash
docker context create eetr01 --docker host=ssh://eetr01
docker context use eetr01
docker info --format '{{.Name}} {{.DockerRootDir}}'
```

That last command must report `/srv/docker`. If it reports your laptop's Docker
root, the context is not active. `ssh eetr01` has to work with key auth and no
password prompt; the host name comes from your SSH config, so use
`ssh://user@10.0.0.240` if you have no alias for it.

Data belongs on `/srv/datastore`, the dedicated 120 GiB `vg0/lv-datastore`
volume — not `/srv/docker`, which is 20 GiB and holds images and container
layers:

```bash
ssh eetr01 'findmnt /srv/datastore && sudo install -d -m 0755 /srv/datastore'
```

Then create the environment file in your checkout:

```bash
cp databases/.env.example databases/.env
chmod 0600 databases/.env
```

Replace every value. Two passwords is the whole credential story here — one for
the PostgreSQL superuser, one for the MongoDB root user. Generate them with
`openssl rand -base64 36` and keep copies in the password manager, because they
are what the admin application will use.

Compose reads `databases/.env` on your laptop and sends the resolved values
over the SSH connection, so the file itself never lands on the server. Keep it
mode 0600 and out of Git; your checkout is the single source of truth for these
credentials. `DATABASE_DATA_DIR` and the published ports are interpreted on
`eetr01`, which is why `/srv/datastore` is a server path.

This keeps a credential file off the server, but not the credentials: Docker
stores the resolved environment in the container config under
`/srv/docker/containers/`, where `docker inspect` and `/proc/<pid>/environ`
expose it. Anyone with root or docker-group access on `eetr01` can read both
passwords. Treat host access as equivalent to database access.

Both passwords are only read when the data directory is empty. Changing one in
`.env` later does not rotate the existing account; rotate it in the database
itself (`ALTER ROLE ... PASSWORD`, `db.changeUserPassword`) and update `.env`
to match.

## Start

From the repository root on your laptop:

```bash
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml up -d
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml up -d
```

Check that both are healthy and listening:

```bash
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml ps
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml ps
ssh eetr01 "sudo ss -lnt '( sport = :5432 or sport = :27017 )'"
```

Both must show `running (healthy)`, and the ports must be listening on
`0.0.0.0` on the server. `restart: unless-stopped` brings the containers back
after a host reboot, so this is a one-time start.

Every `docker compose` command below assumes the `eetr01` context is active.
Add `--context eetr01` explicitly if you would rather leave your laptop's
context as the default, and `docker context use default` when you switch back.

## Connect

From anywhere on the home LAN, using the host's address:

```bash
psql --host 10.0.0.240 --username replace_postgres_admin --dbname postgres

mongosh --host 10.0.0.240 --port 27017 \
  --username replace_mongo_admin \
  --authenticationDatabase admin --password
```

`10.0.0.240` is the `br0` address reserved for `eetr01`. Let the client prompt
for the password instead of putting it in shell history or a saved URI. The
admin application connects to the same address and ports with the same
credentials.

No application database is created here. PostgreSQL starts with only its
built-in `postgres` database and MongoDB with only `admin`; the admin
application creates whatever it needs. The image ships the `vector` extension,
but extensions are per-database, so run `CREATE EXTENSION vector;` (and
`pgcrypto` if wanted) inside each database after creating it.

## Stop, upgrade, and back up

```bash
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml down
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml down
```

`down` leaves the data directories untouched. Before an image update, read the
upstream upgrade notes and take a dump — a data directory cannot be moved
between incompatible major versions.

The dumps stream back over the SSH connection, so these write to your laptop:

```bash
umask 077
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml exec -T postgres sh -c \
  'pg_dumpall --username "$POSTGRES_USER"' > /BACKUP/PATH/postgres.sql

docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml exec -T mongo sh -c \
  'mongodump --username "$MONGO_INITDB_ROOT_USERNAME" \
    --password "$MONGO_INITDB_ROOT_PASSWORD" \
    --authenticationDatabase admin --archive' > /BACKUP/PATH/mongo.archive
```

Dumps contain everything the databases hold and are not encrypted here. Store
them somewhere protected and test a restore before relying on them.

## Known constraints

MongoDB is pinned to the 7.0 series. MongoDB 8.0 and newer exit immediately on
Linux kernels 6.19+ because TCMalloc violates the kernel's rseq ABI
([SERVER-121912](https://jira.mongodb.org/browse/SERVER-121912)); the guard is
only lifted at kernel 7.0.14 and above. Ubuntu 26.04 reports its kernel as
`7.0.0-NN` no matter which stable patches are backported, so every 8.x image
fails on this host with:

```text
MongoDB cannot start: Linux kernel versions 6.19 and newer has a known
incompatibility with this version of MongoDB.
```

Upgrading the host kernel does not fix this, because the version string stays
`7.0.0`. The 7.0 series carries no such guard and runs fine. It is close to
end of life, so re-test an 8.x image periodically and move up once one starts.

PostgreSQL 18 images expect the data mount at `/var/lib/postgresql` and create
a version subdirectory such as `18/docker` beneath it. Mounting at
`/var/lib/postgresql/data`, the pre-18 convention, makes the container refuse
to start.

## Boundaries

- Single-host services: no replication, no automatic failover.
- No TLS. LAN traffic to these ports is unencrypted; encrypt sensitive values
  in the client if that matters.
- The two accounts are superusers with full access to everything on their
  server. Whoever holds a password holds the database.
- The passwords are readable on `eetr01` through the container environment, so
  root or docker-group access on the host is database access.
- `pgcrypto` provides SQL cryptographic functions only. It does not encrypt
  PostgreSQL files, backups, or connections.

## The admin panel's query console

The panel's PostgreSQL query console runs submitted SQL, and it runs it as the
same account the panel administers the server with: the superuser above. There
is no second credential to configure — the console is served wherever
`ADMIN_POSTGRES_DSN` is.

That is a decision about who the panel is for. Whoever reaches it is already
authenticated as an operator and already creates and drops databases, roles and
users through the pages beside the console. A separate login would not have
narrowed that; it would only have narrowed this one endpoint, at the cost of a
second password to rotate.

What bounds a submitted statement is therefore its shape, not its authority:

- It runs in a `READ ONLY` transaction that is always rolled back, so every
  `INSERT`, `UPDATE`, `DELETE`, and DDL is refused by the server.
- `SET LOCAL statement_timeout`, so a runaway query is killed by the server.
- pgx's extended protocol carries one statement per message, so
  `SELECT 1; DROP TABLE x` is refused rather than run as two.

And be plain about what none of that bounds. A superuser session can reach
outside the database: `COPY (SELECT 1) TO PROGRAM 'id > /tmp/escaped'` runs a
shell command as the `postgres` user on the database host, and the `READ ONLY`
transaction does not refuse it, because it is not a database write. Nor can the
session lower its own floor — `SET ROLE` is reversible by whoever authenticated,
so a submitted `RESET ROLE` (or `SET ROLE NONE`, or `DO $$ BEGIN RESET ROLE; END
$$`) restores it and `session_user` never changes. Both checked against
PostgreSQL 18. **Access to the console is access to the database host**, on the
same footing as the passwords readable on `eetr01`.

If the panel ever grows viewers as well as operators, this is the endpoint to
give its own login, and the two facts above are why nothing done inside the
session would substitute for one:

```sql
CREATE ROLE panel_query LOGIN PASSWORD 'generate-a-long-one'
  NOSUPERUSER NOCREATEDB NOCREATEROLE;
GRANT pg_read_all_data TO panel_query;
```

No pattern matching over the SQL text anywhere. Comments, CTEs, dollar quoting
and `DO` blocks all defeat one, and a check would suggest a boundary that the
paragraphs above say plainly is not there.

MongoDB is a different shape. There is no read-only transaction to lean on, so
the boundary is what the panel offers: `find` and nothing else — not `aggregate`,
which can write through `$out` and `$merge`, and not `runCommand`. It also
refuses `$where`, `$function`, and `$accumulator` at any depth in a query
document. Checked against MongoDB 7: the server permits all three on a root
connection, including a `$where` nested inside an `$and`, so the panel's check is
the only thing refusing them.

They are a smaller problem than the PostgreSQL one, and worth describing
accurately: `$where` runs in the server's sandboxed JavaScript engine with no
`require`, `process`, `fs`, or `db` in scope, so it is not a route to the host.
What it is is a forced collection scan and a core occupied until the query
deadline — an unbounded loop inside one ran for the full `maxTimeMS` before being
killed. Enough to refuse from a panel button; not the same class of problem as a
shell on the database host.
