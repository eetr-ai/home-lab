# Admin panel chart

Installs the in-cluster administration panel: the Go API that manages the host
PostgreSQL and MongoDB services and reads the cluster, the web panel an operator
signs in to, and — optionally — the assistant that answers questions about it.

It is a separate chart from [platform](../platform/README.md) rather than a
section of it. The platform chart installs the cluster's shared services and its
values schema is strict about what it requires; the panel is an application this
repository owns, with its own release cadence and its own version. `ansible/README.md`
draws the same line: a custom chart belongs here when an application owned by this
repository has a cohesive set of configurable Kubernetes resources.

## What it installs

| Resource | Notes |
| --- | --- |
| `Deployment/admin-api` | Unprivileged, read-only root filesystem, distroless image |
| `Service/admin-api` | ClusterIP on port 80 |
| `ServiceAccount/admin-api` | The API's identity, and what the read-only ClusterRole is bound to |
| `ClusterRole` + binding | Read-only cluster access for the API — see below |
| `HTTPRoute/admin-api` | **Disabled by default** — see below |
| `Deployment/admin-web` | The panel. No ServiceAccount token: it asks the API |
| `Service/admin-web` | ClusterIP on port 80 |
| `HTTPRoute/admin-web` | Enabled — a panel nobody can reach is not a panel |
| `Deployment/admin-agent` | **Disabled by default.** One replica, `Recreate` — see below |
| `Service/admin-agent` | ClusterIP on port 80. No route: the panel proxies to it |
| `PersistentVolumeClaim/admin-agent-data` | The agent's memory and workspace, on the NFS class |

The images always carry the same version. One release tag builds all three and
publishes the chart, so `admin.api.image.tag`, `admin.web.image.tag` and
`admin.agent.image.tag` should match.

The templates are folded by component rather than by resource kind, the same way
the Go code is:

```text
templates/
  _helpers.tpl
  api/    deployment.yaml service.yaml httproute.yaml serviceaccount.yaml rbac.yaml
  web/    deployment.yaml service.yaml httproute.yaml
  agent/  deployment.yaml service.yaml pvc.yaml
```

Helm walks the directory, so nesting costs nothing and everything one half of the
panel needs is in one place.

## Before installing

### Registry access

Nothing to do here, if the cluster was bootstrapped with a registry credential.
The nodes hold it: `ansible/roles/registry_credentials` writes it into the
kubelet's configuration, so every namespace can pull private images without a
Secret of its own. See [the Ansible guide](../../ansible/README.md).

That is a deliberate trade. One credential on each node, rather than a copy in
every namespace that has to be rotated in step. It costs per-namespace scoping —
any pod scheduled on a node can pull anything the key can read — which is why the
key is read-only and bound to a single repository.

If you ever do want one release to pull with its own credential, create a
`kubernetes.io/dockerconfigjson` Secret in its namespace and name it in
`admin.imagePullSecrets`. The installer checks it exists before Helm runs.

### The values file

```bash
cp charts/admin/values.local.yaml.example charts/admin/values.local.yaml
```

Replace the registry path, set the image tag to the version you intend to run, and
fill in the identity provider:

- `admin.api.oidc.issuer` — the OpenID Connect issuer whose tokens the API accepts,
  written exactly as the provider publishes it. It is `required` in the template,
  so leaving it out fails the render: Helm refuses and Kubernetes never sees a
  Deployment. That is the point — the failure lands where you can read it, rather
  than as an unauthenticated API nobody notices.
- `admin.api.oidc.audience` — the value a token must carry in `aud`, normally the
  panel's client id. It is what stops a token minted for another application being
  replayed here. Neither value is a secret; both travel in every authorization
  request, which is why they live in values rather than a Secret.

The provider's signing keys may be served from a different host than the issuer —
eetr-auth publishes its JWKS on a CDN — so that host has to be reachable from the
cluster.
`values.local.yaml` is ignored. The installer refuses to run while any
`example.invalid` placeholder is still present.

### The database credentials

Each managed service is served only when configured, so a panel with no MongoDB
answers 404 for those routes rather than pretending. Turn one on with
`admin.api.postgres.enabled` / `admin.api.mongo.enabled`, and give it the Secret
holding its connection string.

Both strings carry a superuser password, so they are Secrets created outside Helm
— a values file holding one would end up in Git. The installer checks each exists,
with the key the chart names, before Helm runs.

```bash
kubectl create secret generic admin-postgres --namespace admin \
  --from-literal=dsn='postgres://USER:PASSWORD@HOST:5432/postgres?sslmode=disable' \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic admin-mongo --namespace admin \
  --from-literal=uri='mongodb://USER:PASSWORD@HOST:27017/?authSource=admin' \
  --dry-run=client -o yaml | kubectl apply -f -
```

The credentials are the ones `databases/.env` holds — the single superuser account
each server has. `sslmode=disable` and the plain MongoDB URI are correct here:
those servers carry no TLS by design, which `databases/README.md` documents.

Prefer `--from-file` over `--from-literal` if you would rather the connection
string not enter your shell history.

### The panel's identity provider

The panel is a confidential OIDC client of eetr-auth, and it holds the operator's
access token so it can call the API as them — the same way the assistant agent
will later.

Register the client in eetr-auth first (Clients → New Client):

- token endpoint authentication `client_secret_basic`
- scopes `openid profile email`
- redirect URI `https://<admin.web.hostname>/api/auth/callback/eetr`, matched
  exactly — there are no wildcards, and a loopback URI for local development is
  the only exception
- and **grant every operator that client's environment**. eetr-auth answers
  `access_denied` at `/authorize` otherwise, however administrative the account
  is; admins are environment-scoped for OAuth, not exempt.

Then set `admin.web.hostname` and `admin.web.clientId` in the values file, and
create the Secret holding the two values that must not be in Git:

```bash
kubectl create secret generic admin-oidc --namespace admin \
  --from-literal=authSecret="$(openssl rand -base64 32)" \
  --from-literal=clientSecret='THE_CLIENT_SECRET' \
  --dry-run=client -o yaml | kubectl apply -f -
```

`authSecret` seals the session cookie, which is a JWE carrying the operator's
access and refresh tokens — rotating it signs everybody out. `clientSecret` is
what eetr-auth issued when the client was registered, and it is shown once.

`admin.web.writeEmails` narrows who may change anything. Leave it empty and every
operator who can sign in may write, which is right for one person; set it to a
comma-separated list to hand somebody the panel read-only.

`admin.web.replicas` is 1, and raising it needs a change first: sessions are
stateless, but the token refresh is deduplicated per process, so two replicas can
present the same rotating refresh token at once — and eetr-auth's OAuth 2.1 reuse
detection answers that by revoking the whole family.

### Reading the cluster

On by default, and it needs no credential: in the cluster the pod's own
ServiceAccount is one. The chart creates a ClusterRole and binds it.

Cluster-wide, because the panel's job is to say what is running everywhere and a
Role per namespace would mean editing the chart whenever a namespace is added.

`tests/validate-admin-chart.sh` pins every `(resource, verb)` pair the role grants
and fails the build on any it does not expect — scoped to the rule that granted
it, so it can tell `get` on `nodes` from `get` on `nodes/proxy`, or `patch` on
`deployments/scale` from `patch` on `secrets`. Adding a grant means adding the
pair there in the same change.

### Changing workloads

The role is not read-only any more. It holds four write pairs and no others:

| resource | verb | for |
| --- | --- | --- |
| `apps/deployments` | `patch` | rollout restart |
| `apps/statefulsets` | `patch` | rollout restart |
| `apps/deployments/scale` | `update` | replica count |
| `apps/statefulsets/scale` | `update` | replica count |

Neither operation can create or delete anything, and both are things that already
happen without the panel. What a workload *is* still comes from this repository's
Helm releases.

**Read this before adding another write verb.** `patch` on a Deployment is patch
on *any field of it*. RBAC grants verbs on resources and has no way to express
"only this annotation", so anyone who can reach the restart endpoint could, in
principle, change any Deployment's image, env, `securityContext`, or
ServiceAccount. What actually confines it to a restart is one function in the Go
code — `restartPatch` in `internal/kube/scale.go` — not this role.

Two consequences follow, and both are worth acting on:

- **Set `admin.web.writeEmails`.** An unset allowlist permits every operator who
  can sign in, and the API itself does not authorize per endpoint at all — any
  valid token can restart anything. The allowlist in the panel is the only gate.
- **A `ValidatingAdmissionPolicy`** restricting this ServiceAccount's Deployment
  patches to the `restartedAt` annotation path is the real fix. It is not here
  yet; this note is the record that it should be.

DaemonSets are read but never written. One is sized by the nodes it matches, so
there is no scale subresource to update, and a section that could restart what it
cannot resize would be a confusing half-capability.

Set `admin.api.kubernetes.enabled=false` to serve neither the endpoints nor the
role — for running the API somewhere with no cluster to read.

### Live CPU and memory

The dashboard's usage figures come from `metrics.k8s.io`, which is served by
metrics-server — an optional component the platform chart installs. The grant for
it is unconditional and harmless without it: the panel treats a metrics API that
does not answer as a missing reading, and the dashboard says so for that one
figure rather than failing. Capacity and reservations are read from the core API
and are unaffected.

On k3s, which bundles its own metrics-server, set `metrics-server.enabled=false`
in the platform chart instead of installing a second one — two aggregated API
servers cannot both back the same group.

### Node disk usage

Off by default, and the one grant here worth a deliberate decision.

Node disk usage is in neither the Kubernetes API nor metrics-server, so the only
source is the kubelet's own stats endpoint, reached through the API server's node
proxy. Setting `admin.api.kubernetes.nodeStats.enabled=true` adds `get` on
`nodes/proxy` and sets `ADMIN_KUBERNETES_NODE_STATS` on the pod. That verb reaches
every read endpoint a kubelet serves, including the one that returns files under
`/var/log` on the host. It does **not** permit exec or attach — those need
`create` on the same subresource, which the chart never grants.

Left off, the nodes page reports every other figure and shows the node's
ephemeral-storage allocatable in place of a usage reading, labelled as the
capacity it is.

### The assistant

Off by default, and it costs money per answer, so turning it on is a decision.
It needs one Secret:

```bash
kubectl -n admin create secret generic admin-agent-llm \
  --from-literal=apiKey=sk-or-...
```

...and then `admin.agent.enabled: true` with an image tag. Everything else has a
default: `admin.agent.model` (an OpenRouter model id, vendor-prefixed —
`deepseek/deepseek-v4-flash`), `admin.agent.reasoning`, and
`admin.agent.storage.{className,size}`.

Three things about it are worth understanding before you run it.

**It runs exactly one replica, and that is not an oversight.** The agent is an
[Octo](https://juancavallotti.github.io/octo/) app on Octo's standalone runtime,
whose object store — the agent's memory and its remembered facts — is a map held
in the process and serialized to `OCTO_STORAGE_DIR`. It is read once at startup
and written back a namespace at a time, so two replicas would each answer from
their own copy and each overwrite the other's file. That is also why the
Deployment is `Recreate`: a rolling update runs two pods for a few seconds, which
is the same problem for a shorter time. Sharing the store across replicas needs
the runtime's Kubernetes services provider, which needs the whole Octo platform.

The claim is `ReadWriteMany` on `nfs-client` anyway, and for a different reason:
the store has to be where the pod lands, so a pod rescheduled onto another node
has to find it. RWX is also what keeps raising the replica count a values change
once there is somewhere shared to put the store, rather than a storage migration
as well. **Note the class's reclaim policy: deleting this PVC deletes the
conversations with it.**

**It reads the API as whoever is asking.** The panel's own route handler attaches
the signed-in operator's bearer token to every chat request, and the agent's
`admin_read` tool sends it on. So there is no service identity, no
client-credentials grant and no standing token — and the agent can read exactly
what the operator could read from the panel itself. The tool is `GET`-only, which
is enforced by the runtime rather than by the prompt.

**It is a privileged pod.** It carries `curl`, `jq` and GNU `find`, and a
workspace it can write to, because an assistant that can fetch a health endpoint
and read the JSON back is worth more than one that can only describe how. The
allow list on those tools keeps the ordinary path predictable; it is not a
boundary. The boundary is the pod: it holds the OpenRouter key, it reaches
anything it can route to, and it carries **no ServiceAccount token at all**. If
its network reach matters on your cluster, a NetworkPolicy is the control.

The drawer is gated by `admin.web.writeEmails` — the same allowlist every other
change goes through. An operator given the panel read-only sees no launcher.

## Install

```bash
export KUBECONFIG=ansible/artifacts/admin.conf
task admin:install -- --values charts/admin/values.local.yaml
```

The installer creates the `admin` namespace, applies the
`home-lab.example/gateway-access=true` label the platform Gateway requires before
it will admit a route from this namespace, verifies every pull secret the chart
references actually exists, and then upgrades the release with
`--rollback-on-failure`.

Checking the Secret first is the point of the preflight: a missing one is an
`ImagePullBackOff` several minutes later, with nothing in the event pointing at
the cause.

## The routes

The panel's route is on: `admin.web.hostname` is published through the platform
Gateway, and **Cloudflare Access belongs in front of that hostname**. eetr-auth
authenticates whoever arrives, but the root README lists Access in front of
administrative routes as a final-check requirement, and it is the layer that stops
a stranger reaching the sign-in page at all.

`admin.api.route.enabled` is `false`, and it stays that way until something needs
to reach the API from outside the cluster. The panel does not: its server talks to
the API over the cluster network, at `http://admin-api.admin.svc.cluster.local`.

The caller that will need it is an agent, which does not exist yet. When it does,
put a Cloudflare Access policy in front of the hostname **before** enabling the
route. The API authenticates its callers, but an administrative endpoint should
not be the only thing between the public internet and the cluster.

## Upgrading and uninstalling

Upgrading is the same command with a new image tag. Rolling back is
`helm rollback home-lab-admin --namespace admin`.

```bash
helm uninstall home-lab-admin --namespace admin
```

Uninstalling removes the panel and nothing it manages: the databases run on the
virtualization host under Docker Compose, entirely outside this release. The
`artifact-registry` Secret survives, because Helm does not own it.
