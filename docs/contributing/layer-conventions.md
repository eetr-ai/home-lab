# Layer conventions

How code in `admin/` is arranged, and where a new piece of it goes.

## Fold by component, not by layer

The top-level split is the **component** — `postgres`, `mongo`, `kube` — and the
layers live inside it. There is no repository-wide `handlers/`, `services/`, or
`repositories/` directory.

```text
admin/api/internal/
  postgres/   handler.go  service.go  repo.go  types.go  errors.go  *_test.go
  mongo/      handler.go  service.go  repo.go  types.go  errors.go  *_test.go
  kube/       handler.go  service.go  client.go types.go errors.go  *_test.go
```

The reason is mechanical rather than aesthetic: a component folded this way can be
lifted into its own service by moving one directory and giving it a `main.go`. Fold
by layer instead and the same extraction means picking one file out of three
shared directories and untangling what the rest of each still needs. We do not
plan to split anything today, and the whole point is that we do not have to decide
now.

**A slice never imports another slice's internals.** If `mongo` needs something
`postgres` has, the answer is a shared package under `internal/` — not an import
across the seam. Two slices that import each other are one slice wearing a
disguise, and neither can move.

## The three layers inside a slice

Each layer has one job, and the boundary between them is what makes the slice
testable without a database.

**`handler.go` owns transport, and nothing else.** It decodes the request,
validates its *shape*, calls the service, and maps the result to a status code. It
does not know SQL, driver types, or business rules. A service is never handed a
raw, unparsed request body.

**`service.go` owns the domain logic.** It receives already-validated, typed
inputs and depends on the repository through an interface it declares itself. It
does not touch `net/http`, and it does not construct driver values. This is the
layer that gets the table-driven tests.

**`repo.go` owns persistence, and nothing else.** It holds the driver — `pgx`, the
Mongo client, `client-go` — and translates between driver values and the slice's
own types. No business rules live here.

Errors cross those boundaries as the slice's own error values, declared in
`errors.go`. The handler is the only layer that knows an error is eventually a
`409` or a `404`.

## Declare interfaces where they are consumed

The service declares the narrow interface it needs; the repository satisfies it
without importing it. That keeps the dependency pointing inward and lets a test
supply a fake without a mocking framework.

Do not add an interface for something with exactly one implementation and no test
that needs to substitute it. A speculative interface is indirection that has to be
read through forever in exchange for flexibility nobody asked for.

## The web side: action, client, wrapper

The Next.js app under `admin/web` has its own three layers, and the rule is the
same — never skip straight to `fetch`:

```text
server action  →  typed domain client  →  fetch wrapper
(auth boundary)   (no HTTP verbs)         (the only place that calls fetch)
```

- The **fetch wrapper** is the only code that touches `fetch`. It is one
  `RestClient` from `@eetr/ts-rest-utils`, configured once, whose `authProvider`
  supplies the signed-in operator's bearer token — so authentication is a
  property of the client rather than something each call has to remember. It
  returns a discriminated `ActionResult<T>` — `{ ok: true, data } | { ok: false,
  error }` — and **never throws**.

  It imports `server-only`, and so does anything else that can read the
  operator's token. That module makes importing it from a client component a
  build failure, which is the difference between a rule and a rule that holds.
- The **typed domain client** exposes domain functions (`listDatabases()`,
  `dropRole()`), never HTTP verbs. Paths, methods, base URLs, and headers stay
  inside it. Its modules mirror the API's slices, so the vertical seam runs from
  the browser to the database.
- The **server action** is the trust boundary. It authorizes, then delegates. It
  contains no business logic and never reads an environment variable a client
  component could have read.

**Return a result; do not throw across the boundary.** Next.js redacts thrown
server-action messages in production, so an action that throws gives the user a
generic failure and gives you nothing to debug. Actions return `ActionResult`, and
the browser side unwraps it back into value-or-throw at the last moment.

**Server actions are the default** for reads and mutations alike. Reach for a route
handler only when an action genuinely cannot serve: streaming responses, endpoints
a third party must call by URL, and framework endpoints such as Auth.js.

**A mutation revalidates its whole section, not its own page.** Creating a
PostgreSQL role changes what the databases tab can offer as an owner; dropping a
database changes which extensions can be listed. Refreshing only the page that
acted leaves a sibling tab quietly stale, so the actions call
`revalidatePath("/postgres", "layout")` and let Next refresh what is on screen.

**Server Components fetch; Client Components interact.** The page is a Server
Component that awaits the actions and hands rows down; the client half owns only
what needs a browser — the create panel, the delete confirmations, the scope
picker.

Watch the boundary when passing props: a function cannot cross it, and an icon is
a function. A shared presentational component that a Server Component renders must
therefore *not* be marked `"use client"`, or passing it an icon fails at render
time — which the build does not catch, because it only happens when the page runs.
Such a component imports UI primitives by module rather than through the barrel,
for the reason [ux-guidelines.md](ux-guidelines.md) gives under "The component
library".

## The third module: `admin/agent`

The assistant is neither a slice of the API nor part of the panel. It is an
[Octo](https://juancavallotti.github.io/octo/) app — one `config.yaml`, some
Markdown skills, and a Dockerfile — deployed by the same chart and built by the
same release tag. See [admin/agent/README.md](../../admin/agent/README.md).

The seam is worth stating because it is easy to put something on the wrong side
of it:

- **Anything the agent may *do*** is a tool in `config.yaml`. Not an endpoint in
  the API, not an action in the panel.
- **Anything the agent should *know*** is a skill under `admin/agent/skills/`,
  loaded on demand — not another paragraph of prompt, which is paid for on every
  model call.
- **Anything the browser does with the answer** is in
  `admin/web/src/components/agent/`, and the boundary lives there rather than in
  the definition. `parseNavigateEvent` is the example: the agent asks to navigate,
  and what it is actually allowed to navigate to is decided by the panel, whatever
  the definition says.

The panel reaches it through a **route handler**, not an action, for the reason
stated below: it streams. The handler is also the trust boundary — it authorizes,
overwrites the identity in the request body, and attaches the operator's token —
so it plays exactly the part an action plays elsewhere.

## Where new code goes

| You are adding | It goes in |
| --- | --- |
| A new thing the panel can manage | a new slice under `admin/api/internal/`, plus a matching client module and route group in `admin/web` |
| A new operation on an existing thing | that slice's service, then its handler, then the client module |
| Something two slices both need | a shared package under `internal/` — never a cross-slice import |
| A new client-state domain in the UI | a reducer module built with `@eetr/react-reducer-utils` |
| A typed REST client | `@eetr/ts-rest-utils`; do not hand-roll a second fetch abstraction |
| Something the assistant may do | a tool in `admin/agent/config.yaml` |
| Something the assistant should know | a skill under `admin/agent/skills/`, and a line in the agent's `skills:` list |

## Complete refactors over compatibility shims

When a change improves the design, update every call site, test, and document in
the same change. No deprecated aliases, no dual code paths, no "old and new for
now". Nothing outside this repository depends on these interfaces, so there is no
stability guarantee to preserve — only the cost of carrying two shapes at once.

## File size

Keep files small and focused, one concern each. The web app lints at 200 lines
(blank lines and comments excluded) and CI fails on the warning, so it is a real
bound.

It is a proxy, not the goal. A file goes over because it is doing more than one
thing, and the fix is to find the seam rather than to hit the number. Two splits
recur and work: pulling a data lifecycle out of a component into a hook, and
pulling pure helpers into their own module so they can be tested without React. A
split that leaves the code harder to follow is worse than the warning.
