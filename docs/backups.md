# Database backups to Google Drive

The admin panel schedules backups of the lab's PostgreSQL and MongoDB databases
into a Google Drive folder you pick, and keeps as many of them as you ask for.
This guide covers the one-time Google setup and what the panel does with it.

## How it fits together

```text
Backups page ── connect ──▶ Google consent (drive.file) ──▶ grant kept in a Secret
      │
      └── schedule ──▶ CronJob in the panel's namespace
                         ├─ dump containers   pg_dump / mongodump into a scratch volume
                         └─ upload container  admin-api backup-upload: upload, then trash
                                              whatever is past "keep N"
```

- **The panel sees one folder.** The scope is `drive.file`: the panel reaches the
  files it uploaded and the folders you picked for it in Google's Picker, and
  nothing else in your Drive.
- **Each schedule is a CronJob.** Listing schedules means listing CronJobs, and
  listing runs means listing their Jobs. There is no other record to drift.
- **Retention only touches the panel's own files.** Every upload is tagged with
  the schedule and database it came from, and pruning only considers tagged files.
  Pruned backups go to Drive's trash, which empties itself after 30 days.

## One-time Google setup

Everything below is in the [Google Cloud console](https://console.cloud.google.com/)
for the project `terraform/gcp` manages.

### 1. The APIs and the Picker key

Apply `terraform/gcp`, which needs `panel_origin` set in its ignored
`terraform.tfvars` to the origin the panel answers on. It enables the Drive and
Picker APIs and creates a browser key that is accepted only by the Picker API,
and only from that origin and from `https://docs.google.com`, where Google hosts
the Picker's own frame.

```bash
terraform -chdir=terraform/gcp apply
terraform -chdir=terraform/gcp output -raw picker_api_key
terraform -chdir=terraform/gcp output -raw picker_app_id
```

Those two outputs go in the panel's values, as
`admin.api.backups.google.picker.apiKey` and `.appId`.

To use the Picker on the local k3d cluster too, add
`picker_extra_origins = ["http://localhost:3000"]` to the same file and apply
again. The key itself does not change, only where it is accepted from.

### 2. The consent screen

Under **Google Auth Platform → Branding/Audience**:

1. Set the user type to **External**. A personal Gmail account cannot use Internal.
2. Add the scopes `.../auth/drive.file`, `openid` and `email`. All three are
   non-sensitive, so Google does not ask for app verification.
3. **Publish the app to "In production".** This step is the one that matters
   most. An app left in *Testing* gets refresh tokens that expire after seven
   days, so every scheduled backup would start failing a week after you
   connected.

### 3. The OAuth client

Under **Clients → Create client**, choose **Web application**:

- **Authorized JavaScript origins:** the panel's origin, for example
  `https://admin.example.invalid`.
- **Authorized redirect URIs:**
  `https://admin.example.invalid/api/backups/google/callback`. Google matches it
  exactly, with no wildcards.

Store the client in the panel's namespace. This is the only Secret the feature
needs you to create:

```bash
kubectl -n admin create secret generic admin-google-drive \
  --from-literal=clientId=YOUR_CLIENT_ID \
  --from-literal=clientSecret=YOUR_CLIENT_SECRET
```

The API reads it at startup, so restart `admin-api` after creating or changing it.
Until it exists, the Backups page says Drive is not set up.

### 4. Connect

On the Backups page, choose **Connect Google Drive** and approve the consent
screen. The API exchanges the code and keeps the grant in the Secret
`admin-google-drive-token`, which the backup pods read. **Disconnect** revokes it
at Google and deletes the Secret.

If Google ever says the panel did not get offline access, remove the panel under
your Google account's **Security → Third-party access** and connect again.

## Schedules

A schedule names some databases, a cron line with an optional time zone, a
folder, and how many backups of **each database** to keep. Saving one creates
the CronJob `backup-<name>` in the panel's namespace.

Each run does three things:

1. **Dump.** One init container per engine dumps the chosen databases into a
   scratch volume on the node (`admin.api.backups.job.workSize`, 10Gi by
   default). It uses the same images the servers run, so the tools match:
   - PostgreSQL: `pg_dump --format=custom` for each database, plus
     `pg_dumpall --globals-only` for the roles.
   - MongoDB: `mongodump --archive --gzip` for each database.
2. **Upload.** Each dump lands in the folder as
   `<schedule>-<engine>-<database>-<UTC time>.<ext>`, tagged with the schedule
   and database.
3. **Prune.** For each database that uploaded successfully *in this run*,
   anything beyond the newest N goes to the trash. A database that failed keeps
   all its history until a run succeeds again.

A failed dump or upload fails the run, and the pod's log says which database it
was. Runs never overlap and are never retried automatically. A finished run
stays readable for a week (`admin.api.backups.job.ttlSecondsAfterFinished`), but
Kubernetes also keeps only the last five successful and five failed runs of a
schedule, so on a schedule that runs more than a few times a week the older ones
go sooner. **Run now** starts one immediately, even on a suspended schedule.

Deleting a schedule deletes the CronJob and its runs. **The backups in Drive
are kept.**

## Restoring

### From the panel

Every database backup under *Backups in Drive* on a schedule's page has a
**Restore** button. The restore runs as a Job, listed under *Runs* like a
backup, and its log is there too. It's refused while any run of the schedule
is going.

- **A new database is the default.** The backup is restored into
  `<database>_restored_<date>`, or a name you give. The live database is left
  as it was until you've looked at the copy. A name that already exists is
  refused.
- **Replacing the original** asks you to type the database's name. The
  database is dropped, with its open sessions ended, then created again with
  the owner it had, and the backup is restored into it. Anything written since
  the backup is lost.
- **Roles (PostgreSQL):** any role the backup names that the server lacks is
  created without a login or a password. Existing roles are never touched, so
  no live password is reset. Give a new role a password yourself if anything
  needs to sign in as it.

The roles file (`@globals`) has no button. Restoring a database creates the
roles that database needs.

### By hand

For anything the button doesn't cover, such as rebuilding a server from
nothing, restore by hand. Start with a scratch server, check it, and only then
touch the real one.

1. **Download the backup.** Find it on the schedule's page under *Backups in
   Drive*, or in the folder itself. Trashed backups can be restored from Drive's
   trash for 30 days.
2. **Restore it somewhere disposable first.** These use the same images the
   servers run. The scratch server is empty, so replaying every role is fine
   here, and it lets the database keep its owners, which step 3 reads back.
   `docker run -d` returns before the server accepts connections, so each
   restore waits for it first:

   ```bash
   # PostgreSQL: the roles, then the database (custom format)
   docker run --rm -d --name restore-check -e POSTGRES_PASSWORD=check \
     pgvector/pgvector:0.8.6-pg18-bookworm
   until docker exec restore-check pg_isready -h 127.0.0.1 -U postgres >/dev/null; do sleep 1; done
   gunzip -c nightly-postgres-@globals-*.sql.gz |
     docker exec -i restore-check psql -U postgres
   docker exec restore-check createdb -U postgres app
   docker exec -i restore-check pg_restore -U postgres -d app \
     < nightly-postgres-app-*.dump

   # MongoDB: one database per archive
   docker run --rm -d --name restore-check-mongo mongo:7.0.40-jammy
   until docker exec restore-check-mongo mongosh --quiet --eval 'db.adminCommand("ping").ok' \
     >/dev/null 2>&1; do sleep 1; done
   docker exec -i restore-check-mongo mongorestore --archive --gzip \
     < nightly-mongo-events-*.archive.gz
   ```

3. **Look at the data, then restore for real.** A Docker context moves only
   `docker` commands, not `pg_restore` or `mongorestore` on your laptop. So, as
   in `databases/README.md`, every command below runs **inside the lab's own
   containers**, with the Docker context pointed at the host, and the archive
   streams in over stdin:

   ```bash
   pg() {
     docker compose --env-file databases/.env -f databases/postgres.compose.yaml \
       exec -T postgres sh -c "$1"
   }
   ```

   - **Roles: create only the ones that are missing.** Do **not** replay the
     globals dump against the live server. It holds every role with its password
     verifier as of that night, and replaying it would reset the passwords of
     roles that exist today. Replaying the whole file is only for rebuilding a
     server from nothing.

     The archive needs every role that owns one of its objects, is granted on
     one, or appears in its default privileges. The scratch server has the
     archive restored with its owners, so its catalog can list them, quoted
     and mixed-case names included:

     ```bash
     docker exec -i restore-check psql -U postgres -d app -At <<'SQL'
     SELECT DISTINCT r.rolname FROM pg_roles r JOIN (
       SELECT relowner AS oid FROM pg_class
       UNION SELECT nspowner FROM pg_namespace
       UNION SELECT proowner FROM pg_proc
       UNION SELECT typowner FROM pg_type
       UNION SELECT (aclexplode(relacl)).grantee FROM pg_class
       UNION SELECT (aclexplode(nspacl)).grantee FROM pg_namespace
       UNION SELECT (aclexplode(proacl)).grantee FROM pg_proc
       UNION SELECT defaclrole FROM pg_default_acl
       UNION SELECT (aclexplode(defaclacl)).grantee FROM pg_default_acl
     ) used USING (oid)
     WHERE r.rolname NOT LIKE 'pg\_%' AND r.rolname <> current_user ORDER BY 1;
     SQL
     ```

     Create those that the live server lacks, then set their passwords as you
     would for any new role. Their old definitions are in the globals file, if
     you need to see what one used to be allowed to do.
   - **The database: drop it and create it again**, owned by its original
     owner, then restore **without** `--no-owner`, so every object keeps the
     owner it had. `--force` ends the sessions still open on it, the panel's
     own included, which would otherwise make the drop fail with "database is
     being accessed by other users":

     ```bash
     pg 'dropdb -U "$POSTGRES_USER" --force app && createdb -U "$POSTGRES_USER" -O app_owner app'
     pg 'pg_restore -U "$POSTGRES_USER" -d app' < nightly-postgres-app-*.dump
     ```

     `pg_restore --clean` is not a substitute. It drops only the objects that
     are in the archive, so anything created since the backup is left behind,
     mixed in with the restored data.
   - **MongoDB:** `--drop` replaces each collection it restores, and leaves
     collections that are not in the archive alone. To replace the whole
     database, drop it first.

     ```bash
     docker compose --env-file databases/.env -f databases/mongo.compose.yaml \
       exec -T mongo sh -c 'mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" \
         --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin \
         --eval "db.getSiblingDB(\"events\").dropDatabase()"'
     docker compose --env-file databases/.env -f databases/mongo.compose.yaml \
       exec -T mongo sh -c 'mongorestore --username "$MONGO_INITDB_ROOT_USERNAME" \
         --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin \
         --drop --archive --gzip' < nightly-mongo-events-*.archive.gz
     ```

The globals dump holds every role, with its password verifier. It is as
sensitive as the data, and so is every file in the folder: keep the folder
private.
