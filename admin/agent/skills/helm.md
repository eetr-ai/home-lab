# Helm releases

The cluster tells you what is running. Helm tells you what put it there, and it is
the only thing that does — a Deployment carries no memory of the chart and version
it came from.

## Where the truth lives

There is no database behind any of this. A release, its values, and every previous
revision are Secrets in the namespace it was installed into. Two consequences
worth holding on to:

- A release installed by hand from somebody's laptop shows up in the panel with
  nothing having recorded it. Nothing "registers" a release.
- The panel can only see releases in the namespaces this lab named as Helm
  targets. A release in any other namespace is invisible here — not missing, not
  broken, just outside what the panel was given access to. `GET /api/helm/releases`
  answering `501` means no namespace was named at all.

## Reading a release

| Path | |
| --- | --- |
| `GET /api/helm/releases` | Every release in every managed namespace. |
| `GET /api/helm/namespaces/{ns}/releases` | The releases in one of them. |
| `GET /api/helm/namespaces/{ns}/releases/{release}` | One release, with the values it was given. |
| `GET /api/helm/namespaces/{ns}/releases/{release}/history` | Its revisions, newest first. |
| `GET /api/helm/charts` | What this lab is allowed to install. |

The values on a release are the ones somebody supplied, not the chart's defaults
merged with them. So an empty `values` object means the release runs the chart as
it ships, which is a perfectly ordinary thing for it to say.

## What the statuses mean

- **`deployed`** — the current revision, and its pods came up. Helm waits for
  them, so this is stronger than "the manifests were accepted".
- **`failed`** — it did not. The `description` field is where the reason is, and it
  is usually the useful sentence in the whole response.
- **`superseded`** — a revision that a later successful one replaced. A history
  full of these is a healthy history, not a list of problems, and it is worth
  saying so when somebody is alarmed by a long list.
  Not every old revision is superseded, though: a revision that failed stays
  `failed` in the history forever, and one interrupted part-way stays `pending-*`.
  So read the statuses rather than assuming everything below the current row is
  fine.
- **`pending-install` / `pending-upgrade` / `pending-rollback` / `uninstalling`** —
  something is happening right now. Read it again in a few seconds.

### A pending release that is not going anywhere

A pending release may simply be busy. The API gives each operation
`ADMIN_HELM_TIMEOUT` to finish — ten minutes unless this lab set otherwise — and a
chart that waits for a database to come up can use a good deal of it. Under that,
the honest answer is "it is still running, read it again shortly".

Past it, the operation behind it is almost certainly dead: the API pod was
restarted part-way through, most likely during a rollout. Helm's storage has no
timeout of its own, so the release stays pending, and further installs and
upgrades are refused because the previous operation was never marked done.

Say that plainly when you see it, and say what clears it: rolling back to a
revision from the history. Rollback is deliberately still permitted from a pending
state — it is the recovery path, not another thing that will be refused.

## Revisions count up

Rolling back does not return to the revision it names — it records a *new* one on
top. So rolling a release currently at revision 4 back to revision 2 leaves it at
revision 5, carrying revision 2's configuration.

Two things follow. A rollback can itself be rolled back. And the number never goes
down, which is worth saying to somebody reading a history and expecting it to —
what identifies a revision is its number, not its position.

## What you do not do

**You do not install, upgrade, roll back, or uninstall anything.**

Be clear about why, because it is not that you cannot. `admin_api` will send a
POST, PUT or DELETE to any path under `/api`, and the Helm write endpoints are
under `/api`. If the operator you are calling as holds `admin:deploy`, a request
you make will go through.

So this is a rule you keep rather than a wall you are behind. You call the API as
the person talking to you, which means a deploy you make is a deploy *they* made
without deciding to. Changing what runs on this cluster is a decision somebody
takes at a screen, having seen what they are about to change.

When the answer is a deploy, the answer is the page. The releases live at
`/helm/releases`, one release at `/helm/releases/{namespace}/{name}` with its
upgrade and its history, and what can be installed at `/helm/catalog`. Offer to
take them there with `navigate_to`, and say what you would press.

## The catalog is an allowlist, not a search

`/api/helm/charts` is not a listing of everything installable in the world. It is
the list somebody wrote into this lab's values file, and it is the only thing the
panel will install. So "that chart is not in the catalog" is a complete and
correct answer to "can we install X" — the fix is an edit to the chart's values
and a redeploy, not something anyone can do from the panel.

An entry may come back marked unavailable, which means its repository could not be
reached and the versions shown are only the ones configuration pinned. Say so
rather than reading the short list as the whole truth.
