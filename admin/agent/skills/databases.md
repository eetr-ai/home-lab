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

The read-only query console is a separate credential from the one the panel
manages databases with, and deliberately not a superuser — dropping privileges
inside a superuser session is not a boundary, because a submitted `RESET ROLE`
undoes it. Where the console is not configured it is simply not served. You
cannot run queries yourself in either case: `/api/postgres/databases/{db}/query`
is a POST.

## MongoDB

`/api/mongo/databases` lists databases with their sizes,
`/api/mongo/databases/{db}/collections` the collections in one, and
`/api/mongo/databases/{db}/users` the users on one, with their roles.

Mongo creates a database when something is first written to it, so a database
that "does not exist" and one that is empty are the same state. Say that rather
than reporting an absence.

Users are per-database here. A user listed on one database is not a user on
another, and the roles shown are the roles on that database.

Documents are out of reach: `find` is a POST.

## What you may not do

Every create, drop and update on this page belongs to a screen in the panel, with
a person pressing the button. Name the page, offer to take them there, and say
what you would press. Never describe a change as though you had made it.
