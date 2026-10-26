# Backups to Google Drive

The panel schedules backups of the lab's PostgreSQL and MongoDB databases into
one Google Drive folder per schedule. Everything about it lives in the cluster or
in Drive. There is no other record, so the API's answers are the whole truth.

## What is where

- **The connection** is one Google account. `GET /api/backups/google` says
  whether this lab has an OAuth client (`configured`), whether an account is
  behind it (`connected`), and which (`account`). The grant itself never comes
  back, and you should not look for it. It sits in a Secret only the backup pods
  read.
- **A schedule** is a CronJob named `backup-<name>` in the panel's namespace.
  `GET /api/backups/schedules` lists them:
  - cron line and zone
  - databases (`engine` + `name`)
  - folder
  - `keep`, which is how many backups **of each database** are kept
  - `suspended`
  - `lastScheduled` and `lastSucceeded`
- **A run** is one of its Jobs. `GET /api/backups/schedules/{name}/runs`
  returns them newest first, each with a `phase`, a `pod`, its `namespace` and
  its `containers` in the order they ran.
- **The backups themselves** are files in the Drive folder.
  `GET /api/backups/schedules/{name}/files` reads the folder itself, so it is
  what is really there.

## How a run goes, and how it fails

1. **Dump.** `dump-postgres` and/or `dump-mongo` init containers dump each
   database into a scratch volume. A database that fails is recorded, and the
   others carry on.
2. **Upload.** The `upload` container uploads every dump to the folder, named
   `<schedule>-<engine>-<database>-<UTC time>.<ext>`.
3. **Prune.** For each database that uploaded successfully **in this run**, it
   moves backups beyond the newest `keep` to Drive's trash.

Any failure fails the run, and its log says which step and which database. To
explain a failed run, read the log of every container in the run's `containers`,
in that order: `dump-postgres` and `dump-mongo`, whichever the schedule has, then
`upload`. Bound every read, as for any log:

```text
GET /api/kubernetes/namespaces/{namespace}/pods/{pod}/logs?container=dump-postgres&tail=200
GET /api/kubernetes/namespaces/{namespace}/pods/{pod}/logs?container=dump-mongo&tail=200
GET /api/kubernetes/namespaces/{namespace}/pods/{pod}/logs?container=upload&tail=200
```

**A failed run can still have pruned.** Retention runs per database, for every
database that uploaded in that run, so a run where one database failed still
trashed the oldest backups of the others. Only the database that failed is left
untouched. Never tell anyone a failed run left the folder as it was.

What the usual failures mean:

| The log says | What it means |
| --- | --- |
| `no google account is connected` | Somebody disconnected Drive. Connect again on the Backups page. |
| `google refused the request` with `invalid_grant` | The grant was revoked or expired. Reconnect. If it happens every week, the OAuth consent screen is still in **Testing**, and docs/backups.md says how to publish it. |
| `Drive answered 404` | The folder was deleted, or the panel lost access to it. Edit the schedule and pick the folder again. |
| a dump error naming a database | That database could not be dumped: it was dropped, renamed, or the server was unreachable. Its older backups are kept. |
| `DeadlineExceeded` as the run's reason | The run took longer than the deadline. |
| the pod was evicted | The dumps outgrew the scratch volume. Raise `admin.api.backups.job.workSize`. |

`lastSucceeded` much older than `lastScheduled` is the quickest sign a schedule
is failing quietly. Say so before anyone asks.

## What you may do, and what belongs to a screen

- **Run now:** `POST /api/backups/schedules/{name}/run`. It answers 409 while
  a run is going. It is a full run, retention included: for each database that
  uploads, the oldest kept backup can move to the trash to make room, even if
  another database in the same run fails. Say that, and that you are starting
  it, then point to the runs.
- **Creating or editing a schedule needs a folder chosen in Google's Picker.**
  The Picker is a dialog in the operator's browser that you cannot open. Take
  them to `/backups` instead, or to `/backups/{name}` for an existing one.
- **Connecting and disconnecting Drive** is a trip through Google's consent
  screen. That is the page's job too.
- **Restoring** is the page's job as well. See below.
- **Deleting a schedule** keeps its backups in Drive. Say that, because people
  expect the opposite.
- **Never** describe pruning as deleting. It moves backups to the trash, which
  Drive empties after 30 days.

## Restoring

Each backup on a schedule's page has a **Restore** button. Take people to
`/backups/{name}` for it rather than restoring yourself:

- A restore into a new database (`<database>_restored_<date>`) is the default,
  and it leaves the live database alone.
- Replacing the original drops it first, and the page asks for its name typed
  back. **Never replace a database through the API on anyone's behalf**, even
  when asked. The confirmation belongs to the person, on the page.
- Missing PostgreSQL roles are created NOLOGIN with no password. Existing roles
  are never changed.

A restore shows under the schedule's runs with `operation: "restore"` and its
`target`. Read its `download` then `restore-postgres` or `restore-mongo` logs,
in that order, to explain a failed one. A new database that already exists is
the most common reason. Rebuilding a whole server is the by-hand section of
docs/backups.md.
