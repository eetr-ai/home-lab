# Admin API

The home lab's administration API. It manages the PostgreSQL and MongoDB services
running on the virtualization host, reads the Kubernetes cluster, and publishes an
OpenAPI description of itself.

That last part is why this is a service rather than logic inside the panel. An
assistant agent needs to be able to look up what the API offers instead of being
told, and a description it can read is the difference.

## Layout

Folded by component, not by layer — see
[layer-conventions](../../docs/contributing/layer-conventions.md). Each slice owns
its transport, its domain logic, and its persistence, and imports nothing from
another slice, which is what would let any one of them be lifted into its own
service later.

```text
main.go                  wiring only
internal/
  http/                  transport helpers every slice shares
  auth/                  bearer-token verification, and who a caller is
  health/                the probe endpoint
  openapi/               the generated description, embedded and served
```

## Authentication

Every endpoint under `/api` requires an OAuth 2.1 bearer access token from the
configured OpenID Connect provider. There are no API keys. The two callers that
matter — the panel's own server, and later an agent — both act on behalf of a
person who signed in, so a second credential type would be another thing that can
leak while proving less about who is asking.

Tokens are verified against the provider's published signing keys, checking
signature, issuer, audience, and expiry. Note that the provider's `jwks_uri` may
point at a **different host** than the issuer — eetr-auth serves its keys from a
CDN — so that host has to be reachable from wherever this runs.

The audience check is what stops a token minted for another application being
replayed here. Leaving `ADMIN_OIDC_AUDIENCE` empty disables it, which the process
warns about at startup.

There is no role or group claim to check: the provider refuses to issue a token
for this client at all unless the user is granted the client's environment. Anyone
holding a valid token for it is an operator here.

| Variable | Required | Meaning |
| --- | --- | --- |
| `ADMIN_OIDC_ISSUER` | yes | Issuer URL, kept exactly as written. A missing one is fatal at startup |
| `ADMIN_OIDC_AUDIENCE` | recommended | The value tokens must carry in `aud`, normally the panel's client id |
| `PORT` | no | Defaults to 8090 |

The issuer is required rather than optional because this is a resource server:
running it without one would leave every endpoint open to anything that can reach
the pod, and that is not a state worth being able to reach by accident.

## Running it

```bash
ADMIN_OIDC_ISSUER=https://auth.eetr.app \
ADMIN_OIDC_AUDIENCE=your-client-id \
  task admin-api:run
```

```bash
curl -s localhost:8090/healthz
curl -s localhost:8090/openapi.json | jq '.paths | keys'
curl -s -H "Authorization: Bearer $TOKEN" localhost:8090/api/whoami
```

`/healthz` and `/openapi.json` need no token. The description says which endpoints
exist, never what data they hold, and requiring a token to learn how to present a
token is a loop.

## The OpenAPI description

Generated from the annotations on the handlers, committed as
`internal/openapi/swagger.json`, and embedded into the binary:

```bash
task admin-api:openapi          # regenerate after changing a handler
task admin-api:openapi:check    # fail if the committed copy is stale
```

The generator is a task dependency, never a module dependency: importing it would
give the binary a second copy of the spec that could disagree with the committed
one.

`task lint` runs the drift check, and two tests in `internal/openapi` compare the
routes actually registered against the routes the document claims. They exist
because the drift check alone cannot catch the more likely mistake — a route added
with no annotation regenerates to byte-identical output, produces no diff, and
passes, leaving the description quietly incomplete. The second test catches the
opposite: a described path nothing serves sends a caller to a 404.

## Tasks

```bash
task admin-api:test          # add -- -race for the race detector
task admin-api:lint
task admin-api:build
task admin-api:tidy
```
