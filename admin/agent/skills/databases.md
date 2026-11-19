# The databases

Load this when the question is about PostgreSQL or MongoDB. It is mostly about
what this panel's model of them *is*, because that is where the wrong answers
come from.

Neither of them runs in the cluster. They run under Docker Compose on the
virtualization host, and the API holds a connection to each. So: no pods, no
StatefulSet, no PVC. If somebody is looking for a database pod, tell them there
is not one rather than showing them an empty list.

Either section answers 404 for every route when this installation did not
configure it. That means "not set up here" — not "there are none".

## PostgreSQL

`/api/postgres/databases` lists the databases with owner and size.
`/api/postgres/roles` lists the roles. `/api/postgres/databases/{db}/extensions`
lists what is installed in one.

Two things about how the panel treats them, both of which change what you should
say:

- **A database is created and dropped, never altered through this panel** — bar
  its owner. So there is no "edit" to point somebody at for anything else; the
  answer is drop and recreate, and dropping a database is not a suggestion to
  make lightly.
- **Passwords never reach the server.** When a role is created here the panel
  derives a SCRAM-SHA-256 verifier on its own side and sends that. So you cannot
  read a password back, there is nowhere it is stored, and "what is the password
  for X" has one answer: it is not recoverable, set a new one.

You can run statements: `POST /api/postgres/databases/{db}/query`, body
`{"sql": "..."}`. It is a POST that only reads, which is why a verb is not a
proxy for "destructive" and why the method is yours to choose.

It runs as the **same superuser** the panel manages databases with, so it sees
everything the rest of the panel sees; a permission error there is a real one.
What stops a statement changing anything is the server: it runs inside a `READ
ONLY` transaction that is always rolled back, under a statement timeout, one
statement per request. Write it as a `SELECT` and expect a refusal if you do not.

To **change** data or schema — seeding rows, an `UPDATE`, a `CREATE TABLE` — use
`POST /api/postgres/databases/{db}/execute`, body `{"sql": "..."}`. It is the
write twin of the query console: the same one statement per request and the same
timeout, but the transaction is **committed** rather than rolled back, so an
`INSERT`, `UPDATE`, `DELETE` or DDL persists. It returns the command tag
(`"UPDATE 3"`), the number of rows affected, and any `RETURNING` rows. This — not
`/query` — is how you seed data; `/query` runs it read-only and refuses the write.
One statement per call, so seed a table with a single multi-row `INSERT` rather
than many, and say what you are about to change before you run it.

Two reads help you get the shape right first. `GET
/api/postgres/databases/{db}/tables` lists the tables and views with their
columns, types and primary keys; `POST /api/postgres/databases/{db}/browse` (body
`{"schema", "table", "pageSize"?, "cursor"?}`) pages a table's rows by its primary
key. Omit `cursor` for the first page, then send back the `nextCursor` the
response gives you for the next; when there is no `nextCursor`, that was the last
page. A view or a table without a primary key has nothing to page over, so it
comes back as one capped page — `truncated` says whether rows were left off.

### Privileges

Roles get row access through grants, and the panel manages them — so "give
`app` access to its tables" is a call, not SQL in the console.

`GET /api/postgres/databases/{db}/roles/{role}/privileges` answers what a role
holds **in that database** (privileges are per database). Each object carries
two lists, and the difference matters when you explain an answer:

- `granted` — granted to this role by name. Only these can be revoked from it.
- `effective` — what it can actually do, which also counts PUBLIC, roles it
  belongs to, ownership, and superuser. A privilege that is effective but not
  granted has to be revoked wherever it was granted; say so rather than
  revoking and reporting success.

`defaults` lists `ALTER DEFAULT PRIVILEGES` entries: privileges objects created
**later** will get — but only objects created by that entry's `owner`. A new
table that "lost" its grants was usually created by a different role.

To change them, `POST` the same path with `{"changes": [...]}`. Every change is
`{"action": "grant"|"revoke", "target", "privileges": [...]}` plus what the
target needs:

| `target` | Also needs | Privileges |
| --- | --- | --- |
| `database` | nothing | `CONNECT`, `CREATE`, `TEMPORARY` |
| `schema` | `schema` | `USAGE`, `CREATE` |
| `table` | `schema`, `objects` (table names) | `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `REFERENCES`, `TRIGGER` |
| `sequence` | `schema`, `objects` | `USAGE`, `SELECT`, `UPDATE` |
| `allTables` / `allSequences` | `schema` | as for tables / sequences — every one that exists **now** |
| `defaultTables` / `defaultSequences` | `owner`; `schema` optional (absent = every schema) | as for tables / sequences |

A field the target does not use is a 400, not ignored. The whole list runs in
one transaction, and the response has the `statements` that ran and the role's
state afterwards; a statement the server refuses (a table that does not exist)
is a 422 with PostgreSQL's message, and nothing was applied. Superusers, the
panel's own account, `pg_*` roles, and the `postgres`/`template*` databases are
refused (403).

Read-write access for an application usually means all of: `CONNECT` on the
database, `USAGE` on the schema, `SELECT, INSERT, UPDATE, DELETE` on the tables,
and `USAGE, SELECT` on the sequences — without `USAGE` on a sequence, inserting
into a serial column fails even though `INSERT` was granted. Add the default
targets, with the role the app's migrations run as for `owner`, if tables
created later should be covered too. The panel's role page
(`/postgres/roles/{role}?database={db}`) does the same from a form.

## MongoDB

`/api/mongo/databases` lists databases with their sizes,
`/api/mongo/databases/{db}/collections` the collections in one, and
`/api/mongo/databases/{db}/users` the users on one, with their roles.

Mongo creates a database when something is first written to it, so a database
that "does not exist" and one that is empty are the same state. Say that rather
than reporting an absence.

Users are per-database here. A user listed on one database is not a user on
another, and the roles shown are the roles on that database.

Documents are reachable through `POST /api/mongo/databases/{db}/find` — a POST
that only reads, which is why the method is yours to choose.

## Changing one

You can create, drop and update all of it, as the operator who is asking. Every
one of these also has a screen in the panel, and when there is no hurry that is
the better answer — name the page, offer to take them there, and say what you
would press.

Dropping is where to be slowest. A dropped database does not come back, the panel
gives no undo, and the backups are somebody else's business. Read the list first
and quote the exact name back before you drop anything; if the request is
ambiguous about which one, ask rather than pick. And never describe a change as
done before you have made it and read what came back.
