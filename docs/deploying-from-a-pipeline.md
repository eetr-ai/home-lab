# Deploying from a pipeline

How GitHub Actions or Cloud Build rolls a Helm deployment forward in this lab
without anybody at a terminal.

The short version: an operator declares the deployment once in the panel, and the
pipeline sends one `PATCH` **to the panel** carrying a chart version and any
overrides it owns. The panel exchanges the pipeline's API key for a token and
calls its own API with it. This page is mostly about the three things that are
easy to get wrong — what the credential is and what it can do, what the overrides
do to the values somebody wrote, and what counts as success.

The deployment's own page in the panel shows the exact request for it, ready to
paste, which is a better starting point than retyping the id out of the URL bar.

Every mutation runs as a Kubernetes Job rather than inside the API's own pods.
That is mostly invisible from a pipeline, and it changes two things that are not:
the `202` names a job whose log is Helm's own output, and deploying the panel
itself no longer needs a flag or a caveat about what `deployed` means.

## What has to be true first

**The deployment has to exist.** A pipeline addresses a deployment by id, and
cannot create one. Declaring a chart — its reference, its namespace, its release
name — is a person's decision made in the panel, and the id is in the URL of its
page. This is deliberate: it means a pipeline can change *what version of a known
thing runs*, and cannot introduce a new thing.

It is not, however, a security boundary. Any token this API accepts can call the
declare endpoint too, and there is currently **no allowlist of chart
repositories** — so a pipeline credential can introduce a chart as easily as it
can upgrade one. Read the note at the top of `charts/admin/values.yaml` about
what is still unbounded, and the credential section below about why nothing
narrows a token today.

**The namespace has to be enrolled.** Enrolling one creates two RoleBindings in
it, onto ClusterRoles the admin chart owns — the panel does that from the
namespaces page, and a namespace it creates is enrolled on the spot. Nothing has
to be reinstalled, which is the point: enrolment used to be a values list
rendered at install time, so adding a namespace meant a chart release and a pod
restart.

All of that is conditional on `admin.api.helm.enabled`, which is off by default.
With it off the chart renders none of these grants, nothing is enrolled — a
namespace created from the panel included — and the enrolment routes answer 501.

`admin.api.helm.namespaces` still exists, and is now only a bootstrap list — the
namespaces enrolled at install time, for the ones that cannot be enrolled from a
panel that is not running yet. Read the comment at the top of
`charts/admin/templates/api/rbac-deploy.yaml` before changing it, and before
switching `admin.api.helm.enabled` on at all: an enrolled namespace is one whose
every Secret the panel can read, and the panel can enrol any namespace that is
not protected.

**The admin API does not have to be reachable, and should not be.** The pipeline
talks to the panel, which is already routable and is already the only thing that
calls that API. `admin.api.route.enabled` stays off — the chart validation asserts
it is off by default, because an administrative endpoint should not be publicly
routable before it has a reason to be, and this is one fewer reason.

If **Cloudflare Access** fronts the panel's own hostname, the pipeline needs an
Access **service token** and must send its headers on every request. Without one,
Access answers with an interactive login challenge and the deploy fails in a way
that looks like a panel problem.

## The credential

An **API key**, issued by eetr-auth against the client the panel signs people in
with, and bound to a user:

```bash
curl -X POST "$ISSUER/api/admin/clients/$CLIENT_ID/api-keys" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"userId":"ci-bot","name":"home-lab deploy pipeline"}'
```

The answer carries `apiKey` — `eak_<keyId>_<secret>` — **once**. It cannot be
recovered afterwards; a lost key is revoked and replaced, not looked up.

Store it as one pipeline secret. That is the whole credential: no client id, no
client secret, no token endpoint in the pipeline's configuration. The panel
exchanges it for a token on every request and never caches one, so **revoking a
key takes effect on the next deploy** rather than whenever an issued token would
have expired.

Two things about *which* client the key belongs to:

- Issued against the **panel's own client**, its token already names the audience
  the API checks — eetr-auth puts the client id in `aud` by default — and nothing
  else needs configuring. This is the normal case.
- Issued against a **separate CI client**, the token names *that* client, and the
  API refuses it. The panel can ask for a different audience through
  `OIDC_AUDIENCE` (wired from `admin.api.oidc.audience` by the chart), but only
  when that value is an **absolute URI**: it travels as an RFC 8707 resource
  indicator, and eetr-auth answers `invalid_target` for a bare client id. A
  client-id audience is therefore not something a second client can be pointed at
  — issue the key on the panel's client instead, or give the API a URI audience
  and register it as a resource.

**No scopes**, because this API reads none. A key with scopes is not wrong, it is
just not consulted.

> If eetr-auth ever supports RFC 8693 token exchange, prefer it: GitHub's own
> workload identity token could be exchanged for one of these and the pipeline
> would hold no long-lived secret at all. That is strictly better than what is
> written here.

### What the token is checked for, and what it is not

The API verifies four things and no more: the **signature**, the **issuer**, the
**audience**, and the **expiry**. A token that passes is a token this issuer
minted for this application and has not yet expired. That is a statement about
authenticity, and it is the only one being made.

**It is not a statement about permission, and today nothing else makes one.** Any
token that passes those four checks can call every route on this API — read a
release, roll one out, delete a namespace, restart a workload. A pipeline's
credential is a fully privileged credential.

That is deliberate rather than unfinished, and the reasoning is worth having
because it is what a scope-based design would have cost. Permissions carried in a
JWT have to be modelled in the identity provider, so every new capability here
becomes a change over there. And they are frozen at issue: a token minted an hour
ago carries whatever was true an hour ago, so a credential you suspect is stolen
cannot be narrowed without waiting it out or revoking wholesale.

Authorization is coming as a module in the platform that decides per subject and
per action, asked fresh on each request — which keeps both of those changeable,
and lets a suspicious request be made to sign in again rather than being trusted
because it was signed before anyone was worried.

Until it lands, treat the pipeline's API key with the seriousness of a credential
that can do anything to this cluster's managed namespaces, because that is what it
is. If that is not acceptable yet, the alternative at the bottom of this page —
letting the pipeline run `helm upgrade` with its own kubeconfig — does not depend
on this API at all.

**The endpoint narrows what a pipeline can say; it does not narrow the key.**
`PATCH /api/v1/charts/{chartId}` accepts one deployment id, a chart version, and
overrides, and there is no way to spell "uninstall" or "delete this namespace"
through it. But the token the panel mints from that key is an ordinary token, and
the same key exchanged by hand reaches every route this API has. The endpoint is a
narrow *door*, not a narrow *credential*.

One more thing changes with an API key rather than a client credential:
**attribution now names a person.** A key is bound to a user, that user is the
token's `sub`, and that is what lands in the version's `createdBy` and on the
Job's actor annotation. The version is still recorded with `source: "ci"`, so the
history distinguishes a pipeline's change from an operator's — but the name beside
it is whoever the key was issued for. Issue keys against a purpose-made account if
that would otherwise read as somebody deploying by hand at 03:14.

## The deploy

One request, to the panel. There is no token step: the key goes on the wire and
the panel does the exchange.

```bash
# Capture the status as well as the body. A 409 or a 503 also returns JSON, and
# feeding it to jq further down yields a null job and a request to /jobs/null.
response=$(curl -sS -w '\n%{http_code}' -X PATCH "$PANEL/api/v1/charts/$CHART_ID" \
  -H "Authorization: Bearer $EETR_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"chartVersion":"6.9.4","valueOverrides":{"image":{"tag":"sha-abc123"}}}')

status=${response##*$'\n'}
accepted=${response%$'\n'*}

case "$status" in
  202) ;;
  409) echo "another operation holds this release: $accepted" >&2; exit 75 ;;
  *)   echo "the deploy was refused ($status): $accepted" >&2; exit 1 ;;
esac
```

`409` is worth its own exit code rather than a generic failure: it means somebody
or something else is mid-deploy, and retrying later is right where retrying a
`400` never will be.

### What the overrides do

They are **merged over the newest declared values**, not substituted for them, and
the result is stored as a new version. Maps merge at every depth; lists replace,
which is Helm's own rule.

So the example above changes `image.tag` and leaves `image.repository`,
`replicaCount`, and everything else the operator wrote exactly as it was. A
pipeline that owns an image tag does not have to know the rest of the
configuration, and cannot erase it by sending an incomplete copy.

**Omitting `valueOverrides` entirely** carries the previous document forward byte
for byte, comments included, and only the chart version changes. That is the right
body for a pipeline that tracks chart releases rather than images — and it is what
this repository's own Cloud Build deploy sends.

An empty `"valueOverrides": {}` means the same thing — the API asks whether there
are any overrides, not whether the field was there — so a pipeline that builds its
body programmatically does not have to decide between omitting a key and sending
an empty one.

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
{"namespace":"lab","release":"podinfo","operation":"rollout","job":"helm-rollout-lab-podinfo-x7k2q","message":"accepted, not performed; ..."}
```

Accepted, not done. Helm waits for the pods to come up, which takes longer than
any HTTP request this API will hold open, so there is no version of this that
answers with the result.

`operation` is `rollout`, `rollback`, or `uninstall` — not `install` or `upgrade`.
Which of those a rollout turns out to be is decided by the Job when the work
starts, from what Helm has then: between this answer and the pod starting, a
release can be uninstalled or appear, and an answer given here would be a second
opinion about something the cluster is about to be asked again.

`job` is a Kubernetes Job in the panel's namespace, and it is what does the work.

## Knowing whether it worked

> **The panel serves the deploy and nothing else.** `PATCH /api/v1/charts/{id}` is
> the whole of its pipeline surface: there is no read-back through it, so a
> pipeline that only has an API key is fire-and-forget and the panel is where
> somebody looks. Everything in this section reads the **admin API directly**, and
> therefore applies only to a lab that has chosen to route it — the one reason
> left to. Adding `GET /api/v1/charts/{id}` is the obvious answer if that starts
> to hurt; it does not exist today.

**Read the deployment back.** There is a job to poll now, and it is still not the
success criterion — see the box below.

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

### The job, and why it is not the check

Every mutation now runs as a Kubernetes Job, and the `202` names it — so capture
it from the deploy above rather than going looking:

```bash
job=$(jq -r .job <<<"$accepted")

curl -sS "$API/api/helm/jobs/$job" \
  -H "Authorization: Bearer $token" \
  -H "CF-Access-Client-Id: $CF_ID" -H "CF-Access-Client-Secret: $CF_SECRET"
# {"name":"...","phase":"running","operation":"rollout", ...}
```

`phase` is `pending`, `running`, `succeeded`, or `failed`.

> **A succeeded job is not a successful deploy.** With `rollbackOnFailure` set, a
> failed upgrade is undone — and the job that undid it succeeded. The completion
> rule above is unchanged and is still the one to use: `status` is `deployed`
> **and** `chartVersion` is what you asked for.

One gap in that rule is worth knowing, because it is invisible until it bites: it
proves the *chart version*, and a deploy that changed only values does not move
one. For a values-only change, check the version this lab recorded instead —
`.versions[0].rolledOutAt` is null until that exact declared version reached the
cluster, and a rollback leaves the newer version unstamped:

```bash
jq -e '.versions[0].rolledOutAt != null' <<<"$deployment"
```

That is the stronger check in general, and the reason the weaker one is still
written above is that most pipelines here move a chart version and the status
comparison is the one that reads at a glance.

What the job is genuinely better at is saying *why*. Its log is Helm's own output,
which used to go to the API pod's log where only `kubectl` could reach it:

```bash
curl -N "$API/api/helm/jobs/$job/logs?follow=true" \
  -H "Authorization: Bearer $token" \
  -H "CF-Access-Client-Id: $CF_ID" -H "CF-Access-Client-Secret: $CF_SECRET"
```

Worth doing in a pipeline: it puts the deploy's own account of itself into the
build output, next to whatever failed.

A `404` with code `no_pod_yet` means the pod has not been scheduled — the normal
state for the first moment of every operation, so retry. A `404` with `not_found`
means the job is gone: finished jobs are removed by the cluster after a day. That
is not a loss of the record. **The log is a view; the record is Helm's release
history and the rollout stamp**, and the pod's log can vanish earlier than the TTL
to kubelet garbage collection anyway.

There is also `GET /api/helm/jobs?namespace=&release=&deployment=`, which is how
something that did not start an operation finds it.

## When a deploy will not start

A `503` means the record of declared deployments could not be reached. The
database is down or unreachable; nothing was changed, and retrying is right.

A `409` means a job is already operating on that release, and it names the job.
Usually that is a second pipeline run, and waiting is the right response;
`GET /api/helm/jobs/<name>` says how far along it is.

The check behind that is a listing of jobs, and it is **not a lock**: two API
replicas can both list, both see nothing, and both create. What actually prevents
a double operation is what always did — Helm refuses an operation against a
release its own storage has left pending.

If a 409 persists with no job to point at, the release is wedged: an operation
interrupted part-way leaves it in a `pending-*` state that nothing clears on its
own, and Helm refuses every later attempt because the previous one was never
marked done. That is rarer than it was — the work no longer dies with an API pod
— but a Job can still be evicted or hit its deadline. Nothing recovers it
automatically, deliberately: a guess about whether somebody else's operation is
still alive is how a release gets corrupted.

Clear it by rolling back to a revision from the release's history, which Helm
permits from a pending state. The panel shows this on the Dashboard tab, on the
release's own page, with the revisions underneath it.

**Unless it was the first install that wedged.** A release stuck on revision 1 has
no earlier revision to return to, so there is nothing to roll back and the panel
offers nothing to click. Uninstall it and roll out again — the release record is
all that is wrong, and uninstalling removes whatever the half-finished install did
create. The declared values are in the database and are not affected.

## Deploying the panel itself

This is the case the feature was asked for, and it used to be the awkward one.
Most of what made it awkward is gone.

**The panel's own namespace has to be enrolled.** It is deployable by policy —
`admin` is refused for *deletion* and permitted for *deploys*, because deleting
it destroys the panel and nothing about upgrading needs that. But it still needs
the bindings, and it is the one namespace that cannot be enrolled from a running
panel on a fresh install, because there is no running panel yet. So name it in
`admin.api.helm.namespaces`, which is what that list is for.

Know what that means: it is the namespace holding the panel's OIDC client secret
and its database connection strings, and a Helm grant there is read and write on
all of them.

**That is the whole list now.** There is no `admin.api.helm.selfDeploy` to turn
on: the RBAC verbs the admin chart needs belong to the Job's ServiceAccount and
are always there. And `deployed` means the same thing for this release as for any
other — the pods came up.

### This repository's own build does it

[cloudbuild.yaml](../cloudbuild.yaml) publishes the images and the chart, and then
optionally rolls this lab's panel onto them. It is **off unless `_DEPLOY=true`**,
and that gate is not a convenience: the deploy calls an endpoint the panel serves,
so the panel already running has to be a version that has one. The build that
first introduces the endpoint cannot use it. Publish, upgrade once by hand, then
turn it on.

It needs three more substitutions — `_PANEL_URL`, `_CHART_ID` (the deployment id
from the panel's URL), and `_API_KEY_SECRET`, a Secret Manager secret holding the
`eak_…` key. The deploy step reads that secret itself rather than declaring it in
`availableSecrets`, so a build with `_DEPLOY` unset never needs the secret to
exist at all — which is what keeps a fork of this repository able to publish.

One ordering detail that is easy to undo if this is ever rearranged: each build
step pushes its own image, and there is no top-level `images:` list. That list is
pushed only *after* every step finishes, so reinstating it would leave the deploy
step asking the cluster to pull tags that do not exist yet. The deploy waits on
all three builds and on the chart publish for the same reason.

### What happens, and what you will see

> **Not yet observed on this lab's cluster.** The mechanism below is what the code
> does and the reasoning is checked — the Job carries no Helm ownership markers,
> which a test asserts, and every other operation has been run end to end against
> a real cluster. What has *not* been done is applying the admin chart from the
> panel and watching this play out. Until somebody has, treat the sequence as the
> design rather than as a report, and keep `kubectl` to hand the first time.

Every Helm mutation runs as a Kubernetes Job in the panel's namespace. The Job is
created imperatively by the API, so **it is not part of any Helm release**: Helm
decides what to delete on upgrade by diffing rendered manifests, and it will not
adopt an object that does not carry its ownership labels. Applying the admin chart
therefore does not touch the Job applying it.

So:

1. The `202` comes back with a job name. The panel opens its stream.
2. The Job applies the chart. `admin-api` and `admin-web` begin rolling.
3. **The panel goes away.** The pod serving the page and the pod proxying the
   stream are both being replaced. The panel shows *reconnecting*; a pipeline's
   `curl` gets a connection reset. **This is not the deploy failing.**
4. The Job carries on, because nothing is replacing it, and Helm inside it waits
   for the new `admin-api` and `admin-web` pods to become Ready. That wait was
   impossible before.
5. The new pods come up, the browser reconnects to the same job, and the panel
   shows the rest of it.
6. The Job writes the rollout stamp and exits. The release is `deployed`, meaning
   the pods came up.

From outside the panel, `kubectl -n admin get jobs` and `kubectl -n admin logs
job/<name>` follow it across the gap.

If the upgrade *broke* the panel, the Job fails, Helm records why on the revision,
and the log is still there — which is strictly better than the old arrangement,
where the process doing the upgrading was the one that died.

**The Job runs the image that is running now, not the one being installed.** The
API templates its own image into the Job it creates. That is the right way round —
the code performing the upgrade is the code that was reviewed and is known to
work — and it means a fix to the runner does not apply to the deploy that installs
it.

### The alternative, which is now worse

Giving the pipeline a kubeconfig and letting it run `helm upgrade` directly still
works, and it used to be the better trade for a pipeline that only deployed the
panel: it kept `selfDeploy` off and got a real readiness wait back.

Both of those reasons are gone. There is no flag to keep off, and the readiness
wait is real here. What is left is a credential to manage and a panel deployed
differently from everything else, with nothing bought for it.

## What a pipeline cannot do

Nothing on this list is enforced against the *caller* — the API does not know who
a caller is allowed to be. These are properties of the endpoint: they hold for
everybody, including you.

- **Name a version range.** `^1.4` and `latest` are refused rather than resolved:
  this repository pins everything, and a constraint means installing whatever
  satisfies it on the day it happens to run.
- **Touch a protected namespace.** `platform-system`, `default`, and anything
  under `kube-` are refused with `403`, enrolling one is refused, and putting one
  in the bootstrap list is a chart render failure rather than a warning. The panel's *own* namespace is not
  in that set: it is refused for deletion and permitted for deploys, which is the
  asymmetry the section above depends on.
- **Send values that are not YAML-encodable, or larger than 256 KiB.** Values end
  up in a Secret, and etcd caps those at about a megabyte.
