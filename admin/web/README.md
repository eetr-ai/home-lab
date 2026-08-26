# Admin panel

The browser half of the administration panel: a Next.js application that signs an
operator in against eetr-auth and calls [the admin API](../api/README.md) as them.

It holds no state and talks to nothing else. Everything it shows comes from the
API over one typed client — with one exception, [the assistant](#the-assistant),
which is a second upstream and a stream rather than a request.

## How a request gets from a click to a database

```text
server action  →  typed domain client  →  RestClient  →  admin API  →  PostgreSQL
(authorizes)      (listDatabases())      (bearer token)
```

Three layers, and no shortcuts through them — the rule is in
[layer-conventions.md](../../docs/contributing/layer-conventions.md).

- **`src/lib/api/http.ts`** is the only code that touches HTTP. It is one
  `RestClient` from [`@eetr/ts-rest-utils`](https://www.npmjs.com/package/@eetr/ts-rest-utils)
  whose `authProvider` lifts the operator's eetr-auth access token off the
  session, so authentication is a property of the client rather than something
  each call remembers. It returns a discriminated `ActionResult<T>` and never
  throws.
- **`src/lib/api/{identity,postgres,mongo,kube}.ts`** are the domain client. They
  expose `listDatabases()` and `dropRole()`, never a verb and a path, and they
  mirror the API's slices so the vertical seam runs from the browser to the
  database.
- **`src/app/actions/`** is the trust boundary. Each action authorizes with
  `withRead` / `withWrite`, then delegates. Actions return results; they do not
  throw, because Next.js redacts a thrown server-action message in production and
  the reason is the only useful part.

## The session, and why it holds tokens

The API is an OIDC resource server: every call carries the operator's bearer
token, and the assistant agent will later carry the same one. So the panel keeps
the access and refresh tokens rather than only a session id.

They live in the Auth.js session cookie, which is a **JWE** — `A256CBC-HS512`,
keyed off `AUTH_SECRET` — so it is opaque to the browser, and any replica can read
it without shared storage. There is no `SessionProvider` and no `useSession` in
this app for the same reason: handing the session to a client component would ship
the token to the browser. `src/lib/api/http.ts` imports `server-only` so that is a
build failure rather than a review someone might miss.

Two things about the refresh are worth knowing before changing it, both because
eetr-auth rotates refresh tokens with OAuth 2.1 reuse detection — presenting a
superseded one cascade-revokes the whole family and signs the operator out
everywhere:

- **A failed refresh is never retried.** The session drops its tokens and goes
  back through sign-in.
- **Concurrent refreshes of the same token collapse into one exchange**
  (`src/lib/auth/refresh.ts`). That single-flight is per process, which is why the
  chart runs one replica. Raising it means moving the lock somewhere shared.

A Server Component cannot write cookies, so a renewal during a page render would
be recomputed on every request. `proxy.ts` runs `auth()` on every navigation and
the server actions can write — between them they cover every path that reaches
the API.

## Running it

The whole stack, against whatever `admin/.env` points at — normally the real
services, since SCRAM verifiers and OIDC discovery have nothing to prove against a
fake:

```bash
cp admin/.env.example admin/.env && chmod 600 admin/.env   # then fill it in
task dev
```

The panel is then at `http://localhost:3000` and the API at `http://localhost:8090`.
The [assistant](../agent/README.md) comes up beside them, which is why the file
above needs an `OPENROUTER_API_KEY`. `task logs` follows all three and `task
stop` takes them down.

For UI work, the faster loop is to keep the containerised API and run the panel
with hot reload against it:

```bash
task dev
DOCKER_CONTEXT=default docker compose -f admin/compose.yaml stop web
ADMIN_API_URL=http://localhost:8090 task admin-web:dev
```

## Checks

```bash
task admin-web:lint        # ESLint, plus the theme guard
task admin-web:typecheck
task admin-web:test
```

`lint` runs `scripts/check-theme.mjs`, which fails the build on a raw Tailwind
color ramp or a legacy radius anywhere outside `theme.css`. That guard is what
keeps the two-tier theme from quietly becoming a pile of one-off colors.

Tests are Vitest in the **node** environment over `src/lib/**`. There are no DOM
tests here, deliberately — see
[testing.md](../../docs/contributing/testing.md).

## The assistant

`src/components/agent/` is the drawer, and it breaks two of this app's rules on
purpose.

**It is a route handler, not a server action.** `src/app/api/agent/chat/route.ts`
proxies to the agent, for the reason the pod-log route already gives: a server
action's return value is serialized as one RSC payload, so there is no way to
deliver a token before the last one arrives. It is still the trust boundary — it
authorizes with the same write allowlist every mutation uses, overwrites the
identity in the request body so a forged one cannot survive, and attaches the
operator's bearer token as a header for the agent to call the API with.

`signal: req.signal` is the load-bearing line. The browser aborting cancels this
fetch, which drops the connection to the agent, which ends the model run. Without
it, closing the drawer leaves a model generating for nobody, at full price.

**It pushes rather than overlays**, so it is a third column of the shell's flex
row and not a `SidePanel`. [ux-guidelines.md](../../docs/contributing/ux-guidelines.md#the-assistant-drawer)
says why, and why it must not acquire a scrim, a focus trap or `role="dialog"`.

The parts worth reading are the ones with no React in them: `events.ts` parses the
SSE stream and refuses anything it cannot use, and `turns.ts` folds those frames
into an ordered transcript — a run that thought, called two tools, thought again
and answered reads as those four things in that order. Both are tested;
`parseNavigateEvent` in particular is a security boundary rather than a formatter,
because what the agent may navigate to is decided here and not in its definition.

`react-markdown` and `remark-gfm` are the one third-party rendering dependency,
and the exception is recorded in the UX guidelines.

## The design system

`src/components/ui/` and `src/app/theme.css` are carried over from `eetr-auth` so
the two applications look like one system. Changes that improve a primitive belong
upstream too.

The barrel at `src/components/ui/index.ts` pulls in the overlays and their hooks,
so a Server Component cannot import it — those import the one or two modules they
need directly.

## Versioning

`package.json` says `0.0.0` and stays there. The version that ships comes from the
release tag, applied to the image and the chart together by `cloudbuild.yaml`; a
number tracked here as well would be a second place to disagree.
