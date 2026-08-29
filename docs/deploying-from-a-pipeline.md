# Deploying from a pipeline

How GitHub Actions or Cloud Build rolls a Helm deployment forward in this lab
without anybody at a terminal.

The short version: an operator declares the deployment once in the panel, the
pipeline sends one `PUT` carrying a chart version and any overrides it owns, and
then polls until it stops being pending. This page is mostly about the three
things that are easy to get wrong — what the token has to carry, what the
overrides do to the values somebody wrote, and what counts as success.

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
- audience: the value in `admin.api.oidc.audience`

That is the whole list. **No scopes**, because this API reads none.

Store the client id and secret as pipeline secrets. Rotate them the way you
rotate anything else.

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

Until it lands, treat the pipeline's client secret with the seriousness of a
credential that can do anything to this cluster's managed namespaces, because
that is what it is. If that is not acceptable yet, the alternative at the bottom
of this page — letting the pipeline run `helm upgrade` with its own kubeconfig —
does not depend on this API at all.

## The deploy

```bash
token=$(curl -sS -X POST "$ISSUER/token" \
  -d grant_type=client_credentials \
  -u "$CLIENT_ID:$CLIENT_SECRET" | jq -r .access_token)

accepted=$(curl -sS -X PUT "$API/api/helm/deployments/$DEPLOYMENT_ID" \
  -H "Authorization: Bearer $token" \
  -H "CF-Access-Client-Id: $CF_ID" \
  -H "CF-Access-Client-Secret: $CF_SECRET" \
  -H 'Content-Type: application/json' \
  -d '{"version":"6.9.4","values":{"image":{"tag":"sha-abc123"}}}')
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

**The panel's own namespace has to be a Helm target.** It is deployable by
default — `admin` is refused for *deletion* and permitted for *deploys*, because
deleting it destroys the panel and nothing about upgrading needs that. But it
still has to be named in `admin.api.helm.namespaces`, or covered by
`allNamespaces`.

Know what that means: it is the namespace holding the panel's OIDC client secret
and its database connection strings, and a Helm grant there is read and write on
all of them.

**That is the whole list now.** There is no `admin.api.helm.selfDeploy` to turn
on: the RBAC verbs the admin chart needs belong to the Job's ServiceAccount and
are always there. And `deployed` means the same thing for this release as for any
other — the pods came up.

### What happens, and what you will see

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
  under `kube-` are refused with `403`, and putting one in the managed list is a
  chart render failure rather than a warning. The panel's *own* namespace is not
  in that set: it is refused for deletion and permitted for deploys, which is the
  asymmetry the section above depends on.
- **Send values that are not YAML-encodable, or larger than 256 KiB.** Values end
  up in a Secret, and etcd caps those at about a megabyte.
