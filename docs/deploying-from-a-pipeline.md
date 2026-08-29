# Deploying from a pipeline

How GitHub Actions or Cloud Build rolls a Helm deployment forward in this lab
without anybody at a terminal.

The short version: an operator declares the deployment once in the panel, the
pipeline sends one `PUT` carrying a chart version and any overrides it owns, and
then polls until it stops being pending. This page is mostly about the three
things that are easy to get wrong — what the token has to carry, what the
overrides do to the values somebody wrote, and what counts as success.

## What has to be true first

**The deployment has to exist.** A pipeline addresses a deployment by id, and
cannot create one. Declaring a chart — its reference, its namespace, its release
name — is a person's decision made in the panel, and the id is in the URL of its
page. This is deliberate: it means a pipeline can change *what version of a known
thing runs*, and cannot introduce a new thing.

It is not, however, a security boundary. There is currently **no allowlist of
chart repositories**, so a token holding `admin:deploy` can also reach the
declare endpoint's sibling routes if it holds `admin:write`. Give a pipeline
`admin:read admin:deploy` and nothing else, and read the note at the top of
`charts/admin/values.yaml` about what is still unbounded.

**The namespace has to be one the panel may deploy into.** Either it is named in
`admin.api.helm.namespaces`, or the lab has set `admin.api.helm.allNamespaces`.
Read the comment at the top of `charts/admin/templates/api/rbac-deploy.yaml`
before changing either; neither is a free choice.

**The API has to be reachable from outside the cluster.** `admin.api.route.enabled`
is off by default and the chart validation asserts that it stays off by default,
because an administrative endpoint should not be publicly routable before it has a
reason to be. Turning it on is a real change in this lab's exposure, not a values
flip in passing:

- Put **Cloudflare Access** in front of the hostname first.
- Give the pipeline an **Access service token**, and send its headers on every
  request. Without one, Access answers the pipeline with an interactive login
  challenge and the deploy fails in a way that looks like an API problem.

## The credential

Register a confidential client in eetr-auth:

- grant type `client_credentials` — a pipeline is not a person and has no browser
  to redirect
- allowed scopes: **`admin:read admin:deploy`, and nothing else.** Not
  `admin:write`, which would additionally let the pipeline declare and forget
  deployments, scale and restart workloads, and delete namespaces.
- audience: the value in `admin.api.oidc.audience`

`admin:read` is not optional, and it is worth saying why rather than leaving it
looking like slack in the scope list. A deploy is two operations: the `PUT` that
starts it, and the polling that finds out whether it worked. The read routes
require `admin:read`, and a token that names scopes is held to exactly the ones it
names — so a pipeline issued `admin:deploy` alone gets its `202` and then **403 on
every poll**, which a naive loop reads as "not finished yet" and waits out. A
deployer that cannot observe its own deploy is not a smaller permission, it is a
broken one.

Store the client id and secret as pipeline secrets. Rotate them the way you rotate
anything else.

> If eetr-auth ever supports RFC 8693 token exchange, prefer it: GitHub's own
> workload identity token could be exchanged for an `admin:deploy` token and the
> pipeline would hold no long-lived secret at all. That is strictly better than
> what is written here.

### How the scope is actually enforced, today

It depends on one setting, and it is worth knowing which way yours is set.

`admin.api.oidc.requireScopes` decides what happens to a token that names **no**
`admin:`-prefixed scopes at all. With it **off**, such a token is unrestricted —
so scopes bound only the callers that declare themselves, which is a real bound
on a pipeline and no bound at all on anything else. With it **on**, a scopeless
token gets nothing.

The chart ships it **on**. That is the right default and it has a prerequisite:
eetr-auth has to issue the panel's own client `admin:read admin:write` first,
because the panel presents its operator's token to this API. Turn it off in your
values file until that is done, or the panel is locked out of its own API — and
turn it back on afterwards, because a scopeless token being unrestricted is the
gap this closes.

Either way, a token that names scopes is held to exactly the ones it names, so
the paragraph above about `admin:read` applies in both settings.

## The deploy

```bash
token=$(curl -sS -X POST "$ISSUER/token" \
  -d grant_type=client_credentials \
  -d 'scope=admin:read admin:deploy' \
  -u "$CLIENT_ID:$CLIENT_SECRET" | jq -r .access_token)

curl -sS -X PUT "$API/api/helm/deployments/$DEPLOYMENT_ID" \
  -H "Authorization: Bearer $token" \
  -H "CF-Access-Client-Id: $CF_ID" \
  -H "CF-Access-Client-Secret: $CF_SECRET" \
  -H 'Content-Type: application/json' \
  -d '{"version":"6.9.4","values":{"image":{"tag":"sha-abc123"}}}'
```

### What the overrides do

They are **merged over the newest declared values**, not substituted for them, and
the result is stored as a new version. Maps merge at every depth; lists replace,
which is Helm's own rule.

So the example above changes `image.tag` and leaves `image.repository`,
`replicaCount`, and everything else the operator wrote exactly as it was. A
pipeline that owns an image tag does not have to know the rest of the
configuration, and cannot erase it by sending an incomplete copy.

**Omitting `values` entirely** carries the previous document forward byte for
byte, comments included, and only the chart version changes. That is the right
body for a pipeline that tracks chart releases rather than images.

A version written by a pipeline is recorded with `source: "ci"` and the client id
that sent it, and appears in the deployment's history beside the operator's own.
Nothing is overwritten, so "what did the pipeline change at 03:14" is a question
the panel answers.

One caveat worth knowing: a version created with overrides has its YAML
regenerated, so the *comments* in the previous document are not carried into it.
The previous version still has them, and the generated one says so in its first
line.

The answer is `202`:

```json
{"namespace":"lab","release":"podinfo","operation":"upgrade","message":"accepted, not performed; ..."}
```

Accepted, not done. Helm waits for the pods to come up, which takes longer than
any HTTP request this API will hold open, so there is no version of this that
answers with the result.

## Knowing whether it worked

There is no job to poll and no job id, because the outcome is not something this
API decides — it is written into Helm's own storage by the operation itself. So
the check is to read the deployment back.

```bash
for _ in $(seq 1 60); do
  deployment=$(curl -sS "$API/api/helm/deployments/$DEPLOYMENT_ID" \
    -H "Authorization: Bearer $token" \
    -H "CF-Access-Client-Id: $CF_ID" -H "CF-Access-Client-Secret: $CF_SECRET")

  status=$(jq -r '.release.status // "none"' <<<"$deployment")
  case "$status" in
    pending-*) sleep 10 ;;
    *) break ;;
  esac
done

version=$(jq -r '.release.chartVersion' <<<"$deployment")
[ "$status" = deployed ] && [ "$version" = 6.9.4 ]
```

**That last line is the point of this page.**

A terminal status is not success. If the deploy was sent with
`rollbackOnFailure`, a failed upgrade is undone and the release ends up
`deployed` — on the chart version it started from. A check that waited only for
the status to stop being pending would report a successful deploy of a version
that never deployed.

So the completion rule is: **`status` is `deployed` *and* `chartVersion` is the
version you asked for.** `rollbackOnFailure` is off by default for exactly this
reason — a release left `failed` is legible to both a person and a `curl` loop —
but the check above is correct either way, which is why it is written this way
rather than relying on the default.

`.state` is the API's own summary of the same comparison — `in-sync`, `drifted`,
`pending`, `not-installed`, or `unknown` — and is worth printing when the check
fails. So is `.release.description`, which carries Helm's account of what happened.

`unknown` deserves its own line: it means the live release could not be read at
all. It is not `not-installed`, and a pipeline that treats it as one will try to
install a second copy of something that is already running.

## When a deploy will not start

A `403` on the poll, right after a `202` on the deploy, means the token is missing
`admin:read` — see the credential section. It is the one failure here that looks
like a stuck deploy and is not.

A `503` means the record of declared deployments could not be reached. The
database is down or unreachable; nothing was changed, and retrying is right.

A `409` means something else is already changing that release. Usually that is a
second pipeline run, and waiting is the right response.

If it persists, the release is probably wedged: an operation interrupted part-way
— most often the API pod restarting during a rollout — leaves the release in a
`pending-*` state that nothing clears on its own, and Helm refuses every later
attempt because the previous one was never marked done. Nothing recovers this
automatically, deliberately: two API replicas plus a guess about whether somebody
else's operation is still alive is how a release gets corrupted.

Clear it by rolling back to a revision from the release's history, which Helm
permits from a pending state. The panel shows this on the Dashboard tab, on the
release's own page, with the revisions underneath it.

**Unless it was the first install that wedged.** A release stuck on revision 1 has
no earlier revision to return to, so there is nothing to roll back and the panel
offers nothing to click. Uninstall it and roll out again — the release record is
all that is wrong, and uninstalling removes whatever the half-finished install did
create. The declared values are in the database and are not affected.

## Deploying the panel itself

This is the case the feature was asked for, and it is the awkward one. Three
things have to be true, and each is a decision rather than a setting.

**The panel's own namespace has to be a Helm target.** It is deployable by
default now — `admin` is refused for *deletion* and permitted for *deploys*,
because deleting it destroys the panel and nothing about upgrading needs that.
But it still has to be named in `admin.api.helm.namespaces`, or covered by
`allNamespaces`.

Know what that means: it is the namespace holding the panel's OIDC client secret
and its database connection strings, and a Helm grant there is read and write on
all of them.

**`admin.api.helm.selfDeploy` has to be on.** This chart renders a ClusterRole, a
ClusterRoleBinding, a Role and a RoleBinding, and the deploy grant holds nothing
in `rbac.authorization.k8s.io` without that flag — so `helm upgrade` of the admin
release fails on four objects. The values comment says at length what turning it
on costs; the short version is that the panel can then hand every permission it
holds to any ServiceAccount, and its own credential becomes the most valuable one
in the lab.

**And "deployed" means something weaker for this one release.** Applying this
chart rolls the panel's own Deployment, which terminates the pod running the
upgrade. Helm records a release as deployed only *after* it finishes waiting for
readiness — so if it waited, the record would never be written, the release would
sit in `pending-upgrade` forever, and Helm would refuse every later operation on
it. One self-upgrade would permanently break self-upgrades.

So the API recognises an operation on its own release and does not wait: the
chart's hooks still run and everything is still applied, but the release is
recorded as soon as the manifests are accepted. The `202` says so. The completion
rule at the top of this page therefore proves less here — status `deployed` and
the right `chartVersion` mean the manifests went in, not that the new pods are
healthy. Check the workload:

```bash
kubectl -n admin rollout status deployment/admin-api --timeout=5m
kubectl -n admin rollout status deployment/admin-web --timeout=5m
```

### The alternative for today, which is not worse

Give the pipeline a kubeconfig and let it run `helm upgrade` against the cluster
directly for the admin chart, and use this API for everything else.

It costs a credential to manage and it means the panel is deployed differently
from everything else — but it leaves `selfDeploy` off, keeps the RBAC reach with
the pipeline (which needs it anyway), and gets a real readiness wait back. If the
only thing the pipeline deploys is the panel, this is the better trade.

### Where this should go instead

Every awkward thing on this page follows from one decision: the API runs Helm
inside its own pods. **It should create a Job and let that do the work**, with the
database credential injected so the Job can read the declared values and record
the rollout.

Three problems disappear rather than being managed:

- **The RBAC grant stops being the API's.** The deploy permissions belong to the
  Job's ServiceAccount, and the API keeps read-only access to the cluster.
  `selfDeploy` stops being a flag that widens the panel's own credential and
  becomes a property of a short-lived pod. That is most of what makes it
  uncomfortable today.
- **Self-upgrade stops being a special case.** Nothing is replacing the process
  doing the upgrading, so the readiness wait can stay on and `deployed` goes back
  to meaning the pods came up — for every release, including this one.
- **The 202 gets something real behind it.** A Job is an object with a status,
  which is a better answer to "did it work" than reading the release back and
  inferring.

Not built. Recorded here because it is the design, and the current arrangement is
a first version rather than the intended one.

## What a pipeline cannot do

- **Declare a deployment**, or point an existing one at a different chart. Both
  need `admin:write`. A pipeline changes the version and the values of something
  a person set up.
- **Name a version range.** `^1.4` and `latest` are refused rather than resolved:
  this repository pins everything, and a constraint means installing whatever
  satisfies it on the day it happens to run.
- **Touch a protected namespace.** `platform-system`, the panel's own namespace,
  and anything under `kube-` are refused with `403`, and putting one in the
  managed list is a chart render failure rather than a warning.
- **Send values that are not YAML-encodable, or larger than 256 KiB.** Values end
  up in a Secret, and etcd caps those at about a megabyte.
