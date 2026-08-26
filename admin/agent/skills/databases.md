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
