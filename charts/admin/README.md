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
