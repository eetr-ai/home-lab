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
permits from a pending state. The panel shows this on the Releases tab with the
history underneath it.

**Unless it was the first install that wedged.** A release stuck on revision 1 has
no earlier revision to return to, so there is nothing to roll back and the panel
offers nothing to click. Uninstall it and roll out again — the release record is
all that is wrong, and uninstalling removes whatever the half-finished install did
create. The declared values are in the database and are not affected.

## What a pipeline cannot do

Nothing on this list is enforced against the *caller* — the API does not know who
a caller is allowed to be. These are properties of the endpoint: they hold for
everybody, including you.

- **Name a version range.** `^1.4` and `latest` are refused rather than resolved:
  this repository pins everything, and a constraint means installing whatever
  satisfies it on the day it happens to run.
- **Touch a protected namespace.** `platform-system`, the panel's own namespace,
  and anything under `kube-` are refused with `403`, and putting one in the
  managed list is a chart render failure rather than a warning.
- **Send values that are not YAML-encodable, or larger than 256 KiB.** Values end
  up in a Secret, and etcd caps those at about a megabyte.
