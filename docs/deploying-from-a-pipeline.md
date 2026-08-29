# Deploying from a pipeline

How GitHub Actions or Cloud Build rolls a Helm release forward in this lab without
anybody at a terminal.

The short version: the pipeline gets a token from eetr-auth, sends one `PUT`
carrying a version, and then polls the release until it stops being pending. This
page is mostly about the two things that are easy to get wrong — what the token
has to carry, and what counts as success.

## What has to be true first

**The chart has to be in the catalog.** `admin.api.helm.charts` is an allowlist,
not a search: the API installs an entry somebody wrote into a values file, and
never a URL a caller chose. A pipeline cannot introduce a chart, and that is the
whole reason installing from one is safe to allow at all. Adding a chart is a
values edit and a redeploy, made by a person.

**The namespace has to be in `admin.api.helm.namespaces`.** Read the comment at
the top of `charts/admin/templates/api/rbac-deploy.yaml` before adding one; it is
not a free choice.

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
- allowed scope: **`admin:deploy` and only that.** Not `admin:write`, which would
  additionally let the pipeline scale and restart workloads; not `admin:read`,
  which it does not need to check its own deploy — reading a release is permitted
  to any accepted token today.
- audience: the value in `admin.api.oidc.audience`

Store the client id and secret as pipeline secrets. Rotate them the way you rotate
anything else.

> If eetr-auth ever supports RFC 8693 token exchange, prefer it: GitHub's own
> workload identity token could be exchanged for an `admin:deploy` token and the
> pipeline would hold no long-lived secret at all. That is strictly better than
> what is written here.

### How the scope is actually enforced, today

Honestly: **partially.** The API refuses a caller whose token names scopes but not
the one a route needs, and a pipeline's token names exactly one — so for the
pipeline this is real authorization. A token naming *no* scopes is still
unrestricted, which is what the panel's own token looks like, and is why
`admin.api.oidc.requireScopes` is off.

Turning `requireScopes` on closes that, and it cannot be turned on until eetr-auth
issues the panel's client `admin:read admin:write`. Until then the honest summary
is that scopes bound the callers that declare themselves, and the pipeline is one.

## The deploy

```bash
token=$(curl -sS -X POST "$ISSUER/token" \
  -d grant_type=client_credentials \
  -d scope=admin:deploy \
  -u "$CLIENT_ID:$CLIENT_SECRET" | jq -r .access_token)

curl -sS -X PUT "$API/api/helm/namespaces/apps/releases/my-app" \
  -H "Authorization: Bearer $token" \
  -H "CF-Access-Client-Id: $CF_ID" \
  -H "CF-Access-Client-Secret: $CF_SECRET" \
  -H 'Content-Type: application/json' \
  -d '{"version":"1.4.2"}'
```

The body carries **only the version**. Values are left out on purpose, and leaving
them out is meaningful: it tells the API to keep the values the release already
has. So a pipeline that owns an image tag does not have to know the rest of the
configuration — and cannot erase it by sending an incomplete copy.

Do not start sending values from a pipeline. The moment two things write them,
the panel's stored values stop being the source of truth and there is nothing
reconciling the two. Versions from CI, values from the panel.

The answer is `202`:

```json
{"namespace":"apps","release":"my-app","operation":"upgrade","message":"accepted; ..."}
```

Accepted, not done. Helm waits for the pods to come up, which takes longer than
any HTTP request this API will hold open, so there is no version of this that
answers with the result.

## Knowing whether it worked

There is no job to poll and no id to poll it with, because there is no database:
the outcome is written into Helm's own storage. So the check is to read the
release back.

```bash
for _ in $(seq 1 60); do
  release=$(curl -sS "$API/api/helm/namespaces/apps/releases/my-app" \
    -H "Authorization: Bearer $token" \
    -H "CF-Access-Client-Id: $CF_ID" -H "CF-Access-Client-Secret: $CF_SECRET")

  status=$(jq -r .status <<<"$release")
  case "$status" in
    pending-*) sleep 10 ;;
    *) break ;;
  esac
done

version=$(jq -r .chartVersion <<<"$release")
[ "$status" = deployed ] && [ "$version" = 1.4.2 ]
```

**That last line is the point of this page.**

A terminal status is not success. If the release was upgraded with
`rollbackOnFailure`, a failed upgrade is undone and the release ends up
`deployed` — on the chart version it started from. A check that waited only for
the status to stop being pending would report a successful deploy of a version
that never deployed.

So the completion rule is: **`status` is `deployed` *and* `chartVersion` is the
version you asked for.** `rollbackOnFailure` is off by default for exactly this
reason — a release left `failed` is legible to both a person and a `curl` loop —
but the check above is correct either way, which is why it is written this way
rather than relying on the default.

`description` carries Helm's own account of what happened, and is the field worth
printing when the check fails.

## When a deploy will not start

A `409` means something else is already changing that release. Usually that is a
second pipeline run, and waiting is the right response.

If it persists, the release is probably wedged: an operation interrupted part-way
— most often the API pod restarting during a rollout — leaves the release in a
`pending-*` state that nothing clears on its own, and Helm refuses every later
attempt because the previous one was never marked done. Nothing recovers this
automatically, deliberately: two API replicas plus a guess about whether somebody
else's operation is still alive is how a release gets corrupted.

Clear it by rolling back to a revision from the release's history, which Helm
permits from a pending state. The panel shows this on the release page with the
history underneath it.

## What a pipeline cannot do

- **Install a chart that is not in the catalog**, or at a version its repository
  does not offer. `404` and `400` respectively.
- **Upgrade a release the catalog does not list.** The panel can see every release
  in a managed namespace and can only change the ones that were vetted.
- **Name a version range.** `^1.4` and `latest` are refused rather than resolved:
  this repository pins everything, and a constraint means installing whatever
  satisfies it on the day it happens to run.
- **Touch a protected namespace.** `platform-system`, the panel's own namespace,
  and anything under `kube-` are refused with `403`, and putting one in the
  managed list is a chart render failure rather than a warning.
