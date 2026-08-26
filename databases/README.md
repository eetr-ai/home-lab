# Host databases

This module runs PostgreSQL and MongoDB directly on the Ubuntu virtualization
host. The containers are independent Compose projects and use Docker named
volumes under the host's configured Docker data root. They do not run in a VM
or in Kubernetes.

The intended network paths are:

```text
Kubernetes node -> eetr01 br0 address -> database container
Operator laptop -> SSH to eetr01 -> 127.0.0.1 -> database container
```

There is no Cloudflare route or other public endpoint. The managed firewall
rules allow only the three configured Kubernetes node addresses to use the LAN
bindings.

## Security and availability boundaries

- PostgreSQL uses SCRAM-SHA-256 password authentication and enables `vector`
  and `pgcrypto` in the initial database.
- `pgcrypto` supplies SQL cryptographic functions. It does not transparently
  encrypt PostgreSQL files, backups, keys, or network traffic.
- MongoDB is an authenticated standalone server. MongoDB Client-Side Field
  Level Encryption is configured by a future client application, not by the
  server Compose file.
- Database TLS and LUKS encryption are not part of this module. Traffic from
  Kubernetes nodes to the host is not encrypted in transit. Encrypt sensitive
  values in the client until database TLS is added.
- These are single-host services without replication or automatic failover.
- Only administrator accounts are bootstrapped. Applications must use
  separate, least-privileged users whose credentials are stored as encrypted
  Kubernetes Secrets.

## Prerequisites

Run these checks on `eetr01`:

```bash
docker compose version
docker info --format '{{.DockerRootDir}}'
findmnt /srv/docker
df -h /srv/docker
docker system df
ip -4 -brief address show br0
sudo iptables -n -L DOCKER-USER
sudo ss -lnt '( sport = :5432 or sport = :27017 )'
```

Docker must use the dedicated mounted filesystem rather than the root
filesystem. The database ports must not already be occupied. The firewall
helper requires Docker's `DOCKER-USER` chain and the `conntrack` iptables
matcher. Images, database volumes, and Docker metadata share the 20 GB Docker
filesystem, so monitor it and alert before it fills. Never reclaim space with
an unreviewed volume-pruning command.

Install persistent firewall support before saving verified rules:

```bash
sudo apt update
sudo apt install -y iptables-persistent
```

## Configure local inputs and Secrets

From the repository checkout on `eetr01`:

```bash
cp databases/.env.example databases/.env
chmod 0600 databases/.env
```

Replace every example value. `DATABASE_LAN_ADDRESS` is the IPv4 address owned
by `br0`, and `DATABASE_ALLOWED_CLIENTS` is exactly the three reserved
Kubernetes node addresses separated by commas. Do not add the operator laptop;
administrative access uses SSH instead.

Create two password files outside the repository and outside the Docker data
volumes. The alternate secure storage location must be mounted before starting
the containers.

```bash
umask 077
install -d -m 0700 /ABSOLUTE/SECURE/PATH/database-secrets
openssl rand -base64 48 > /ABSOLUTE/SECURE/PATH/database-secrets/postgres_admin_password
openssl rand -base64 48 > /ABSOLUTE/SECURE/PATH/database-secrets/mongo_admin_password
chmod 0600 /ABSOLUTE/SECURE/PATH/database-secrets/*
```

Set `DATABASE_SECRETS_DIR` in `databases/.env` to that absolute directory.
Keep recoverable copies of both passwords in the password manager. Compose
mounts the files read-only and passes only their in-container paths. Local
Compose preserves the source file permissions, while MongoDB drops privileges
before reading its password file. The tracked MongoDB wrapper therefore copies
that file into a private container tmpfs with `mongodb` ownership, then
delegates initialization and credential cleanup to the official entrypoint.
The plaintext copy never lands in the container layer or a Docker volume.

Validate the rendered configurations. The output contains usernames, paths,
and addresses, but must not contain password contents.

```bash
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml config --quiet
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml config --quiet
```

## Apply the cluster-only firewall

Apply the rules before starting the databases:

```bash
sudo databases/configure-firewall.sh apply databases/.env
sudo databases/configure-firewall.sh status
```

The helper owns only the `HOME_LAB_DATABASES` chain and one jump to that chain
from `DOCKER-USER`. It matches the original host address and ports after Docker
has performed destination NAT. Other forwarded Docker traffic returns to the
existing rules unchanged.

Test from all three Kubernetes nodes and confirm an unlisted LAN client cannot
connect before making the rules persistent:

```bash
sudo netfilter-persistent save
```

Rollback removes only the managed chain:

```bash
sudo databases/configure-firewall.sh remove
sudo netfilter-persistent save
```

If Docker is changed to its native nftables firewall backend, stop and replace
this iptables-specific helper before exposing the LAN bindings.

## Start and verify PostgreSQL

```bash
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml up -d
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml ps
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml exec postgres \
  psql --username replace_postgres_admin \
  --dbname replace_postgres_database \
  --command='SELECT extname, extversion FROM pg_extension WHERE extname IN ('\''vector'\'', '\''pgcrypto'\'') ORDER BY extname;'
```

The extension initialization script runs only when the PostgreSQL volume is
empty. Changing the SQL file does not modify an existing database.

## Start and verify MongoDB

```bash
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml up -d
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml ps
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml exec mongo sh -c \
  'mongosh --quiet --host 127.0.0.1 \
    --username "$MONGO_INITDB_ROOT_USERNAME" \
    --password "$(cat /run/home-lab-secrets/mongo_admin_password)" \
    --authenticationDatabase admin \
    --eval '\''db.adminCommand({ ping: 1 })'\'''
```

The initialization variables only create the administrator on an empty MongoDB
volume. Changing them later does not rotate an existing account.

Verify host bindings:

```bash
sudo ss -lnt '( sport = :5432 or sport = :27017 )'
```

Each enabled service must listen on `127.0.0.1` and the configured `br0`
address. There must be no `0.0.0.0`, `[::]`, or public-interface binding.

## Administrator connections

Open a tunnel from the operator laptop and leave it running:

```bash
ssh -N -L 15432:127.0.0.1:5432 eetr01
```

Connect `psql`, pgAdmin, or another PostgreSQL client to `127.0.0.1:15432`.
Let the client prompt for the password instead of putting it in shell history.

MongoDB uses a separate local port:

```bash
ssh -N -L 27018:127.0.0.1:27017 eetr01
mongosh --host 127.0.0.1 --port 27018 \
  --username replace_mongo_admin \
  --authenticationDatabase admin --password
```

MongoDB Compass can use the same host, port, username, and `admin`
authentication database. Do not place the password in a saved URI.

Kubernetes applications connect to the configured `br0` address on the native
database ports. The host address may be stored in non-secret application
configuration, but usernames and passwords belong in encrypted Kubernetes
Secrets.

## Stop, upgrade, and recover

Stop a service without deleting its named volumes:

```bash
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml down
docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml down
```

`docker compose down --volumes` permanently deletes the selected database
volume. Do not use it unless a tested backup exists and deletion is intended.

Before an image update, read the upstream upgrade notes, create a logical dump,
and test restoration. Never move a named volume directly between incompatible
major database versions.

Create logical dumps into a protected host directory with sufficient space:

```bash
umask 077
docker compose --env-file databases/.env \
  -f databases/postgres.compose.yaml exec -T postgres sh -c \
  'pg_dumpall --username "$POSTGRES_USER"' \
  > /ABSOLUTE/SECURE/BACKUP/PATH/postgres.sql

docker compose --env-file databases/.env \
  -f databases/mongo.compose.yaml exec -T mongo sh -c \
  'mongodump --username "$MONGO_INITDB_ROOT_USERNAME" \
    --password "$(cat /run/home-lab-secrets/mongo_admin_password)" \
    --authenticationDatabase admin --archive' \
  > /ABSOLUTE/SECURE/BACKUP/PATH/mongo.archive
```

The dumps are sensitive and are not encrypted by this module. Encrypt them in
the backup system, retain the matching credentials and application encryption
keys, and perform a restore test before relying on them.
