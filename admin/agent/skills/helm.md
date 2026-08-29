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
- **`superseded`** — an old revision. Every revision but the current one is
  superseded, so a history full of them is a healthy history, not a list of
  problems.
- **`pending-install` / `pending-upgrade` / `pending-rollback` / `uninstalling`** —
  something is happening right now. Read it again in a few seconds.

### A pending release that is not going anywhere

If a release has been pending for many minutes, the operation behind it is almost
certainly dead — the API pod was restarted part-way through, most likely during a
rollout. Helm's storage has no timeout, so it stays that way, and every later
attempt is refused because the previous one was never marked done.

Say this plainly when you see it, and say what clears it: rolling back to a
revision from the history, which Helm permits from a pending state. Do not suggest
waiting; nothing is going to finish.

## Revisions count up

Rolling back to revision 2 creates revision 5. It does not return to 2. That means
a rollback can itself be rolled back, and it means "revision 5" and "the fifth
change" are the same thing — which is worth saying when somebody is reading a
history and expecting the number to go down.

## What you do not do

**You do not install, upgrade, roll back, or uninstall anything.** You have no
tool for it, and that is deliberate rather than an oversight: you call the API as
the operator who is talking to you, so a tool that could deploy would let a
conversation deploy. Changing what runs on this cluster is a decision somebody
makes at a screen.

So when the answer is a deploy, the answer is the page. The releases live at
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
