# Admin panel chart

Installs the in-cluster administration panel: the Go API that manages the host
PostgreSQL and MongoDB services and reads the cluster.

It is a separate chart from [platform](../platform/README.md) rather than a
section of it. The platform chart installs the cluster's shared services and its
values schema is strict about what it requires; the panel is an application this
repository owns, with its own release cadence and its own version. `ansible/README.md`
draws the same line: a custom chart belongs here when an application owned by this
repository has a cohesive set of configurable Kubernetes resources.

The web interface is not here yet. It arrives with its image, and until then the
chart installs the API alone.

## What it installs

| Resource | Notes |
| --- | --- |
| `Deployment/admin-api` | Unprivileged, read-only root filesystem, distroless image |
| `Service/admin-api` | ClusterIP on port 80 |
| `ServiceAccount/admin-api` | The API's identity; holds no permissions until the cluster-reading slice grants them |
| `HTTPRoute/admin-api` | **Disabled by default** — see below |

## Before installing

### The image pull secret

The images live in a private Artifact Registry repository, so the cluster needs a
credential. Create it from a key for the read-only `home-lab-puller` account that
[terraform/gcp](../../terraform/gcp/README.md) declares — read-only on purpose, so
a leaked key cannot overwrite a published image.

Keys are not managed by Terraform, because Terraform would write the private key
into a local state file. Create one by hand:

```bash
umask 077
gcloud iam service-accounts keys create ./home-lab-puller.json \
  --iam-account "home-lab-puller@YOUR_PROJECT.iam.gserviceaccount.com"

kubectl create namespace admin --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret docker-registry artifact-registry \
  --namespace admin \
  --docker-server=us-west1-docker.pkg.dev \
  --docker-username=_json_key \
  --docker-password="$(cat ./home-lab-puller.json)" \
  --dry-run=client -o yaml | kubectl apply -f -

rm -P ./home-lab-puller.json   # shred -u on Linux
```

The Secret is not owned by Helm, matching how the platform chart's Cloudflare
credentials work. Rotating it is the same two commands with a fresh key, followed
by deleting the old key in Google Cloud.

This is a Kubernetes pull secret rather than registry authentication configured on
the nodes. It is scoped to the one namespace that needs it and rotates with a
single command, and it leaves the Ansible containerd role alone — that role
generates the stock configuration and asserts on its shape, so node-level
credentials would mean re-templating it and putting a key on all three nodes.

### The values file

```bash
cp charts/admin/values.local.yaml.example charts/admin/values.local.yaml
```

Replace the registry path and set the image tag to the version you intend to run.
`values.local.yaml` is ignored. The installer refuses to run while any
`example.invalid` placeholder is still present.

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

## The API route

`admin.api.route.enabled` is `false`, and it stays that way until something needs
to reach the API from outside the cluster. The panel's own browser session does
not: it talks to the API over the cluster network.

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
