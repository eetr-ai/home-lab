# Home lab platform chart

This umbrella chart installs the services that make the kubeadm cluster useful
for applications:

- Traefik `v3.7.11` through chart `41.3.0`.
- cert-manager `v1.21.1` with Cloudflare DNS-01 issuers.
- NFS Subdirectory External Provisioner chart `4.0.18`.
- cloudflared `2026.8.2` for a remotely managed tunnel.
- Redis `8.10.1` as a coordination point.
- An optional whoami `v1.12.0` end-to-end test.

Cilium is not a dependency. It is a cluster bootstrap prerequisite and remains
installed separately from its pinned upstream OCI chart.

## Routing model

The chart uses Kubernetes Gateway API exclusively. It does not enable
Traefik's Ingress or Traefik CRD providers and does not create Kubernetes
`Ingress` resources.

The installer applies the pinned Gateway API v1.5.1 Standard CRDs before Helm.
It passes `--skip-crds` to prevent the Traefik dependency from installing its
unused proprietary CRDs; cert-manager's explicitly enabled CRDs are rendered
as normal chart templates and remain installed.
The chart then creates:

- GatewayClass `traefik`.
- Gateway `home-lab` with HTTP and HTTPS listeners.
- A wildcard TLS certificate through cert-manager.
- `HTTPRoute` resources for enabled platform workloads.

Only namespaces carrying this label may attach routes:

```bash
kubectl label namespace YOUR_NAMESPACE \
  home-lab.example/gateway-access=true
```

## Prerequisites

1. Bootstrap Kubernetes and install Cilium using the
   [Ansible guide](../../ansible/README.md).
2. Configure the NFS export using the
   [generic NFS guide](../../docs/nfs-server.md); keep the real topology in
   private operations notes and ignored local values.
3. Put the public DNS zone in Cloudflare.
4. Create a remotely managed Cloudflare Tunnel.
5. Create a Cloudflare API token with `Zone:DNS:Edit` and `Zone:Zone:Read` for
   only the intended zone.
6. Store the API token and tunnel token in separate mode-`0600` files outside
   this repository.

Anyone who obtains the tunnel token can run a connector for that tunnel.
Treat both files as credentials and keep recoverable copies in a password
manager.

## Create the namespace and Secrets

```bash
export KUBECONFIG="$PWD/ansible/artifacts/admin.conf"
kubectl create namespace platform-system --dry-run=client -o yaml \
  | kubectl apply -f -

kubectl create secret generic cloudflare-api-token \
  --namespace platform-system \
  --from-file=api-token=/ABSOLUTE/SECURE/PATH/cloudflare-api-token \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic cloudflared-token \
  --namespace platform-system \
  --from-file=token=/ABSOLUTE/SECURE/PATH/cloudflared-token \
  --dry-run=client -o yaml | kubectl apply -f -
```

Redis is installed by default, so its password is a prerequisite rather than a
conditional one — the installer refuses to run without it:

```bash
kubectl create secret generic redis-password \
  --namespace platform-system \
  --from-literal=password="$(openssl rand -base64 32)" \
  --dry-run=client -o yaml | kubectl apply -f -
```

These Secrets are not owned by Helm. Helm values contain only their names and
keys.

## Redis

Installed by default, which is an exception among the optional services here: the
others each need something outside the cluster to be true before they are useful,
and this one does not. Switch it off with `platform.redis.enabled: false` on a
cluster that wants nothing coordinated.

It is a **coordination point, not a store**: one replica, no persistence, and a
restart is an empty cache. Both of those are deliberate. A lock has to have a
single authority, so a second Redis is not more availability — it is a second
answer to a question that must have exactly one, and replication would make the
failure it prevents possible again on a split brain. `maxmemory-policy` is
`noeviction` for the same reason: an evicted lock is not a lock, so reaching the
ceiling fails a write loudly instead.

Two controls, not one. The password says who you are; the NetworkPolicy says
where you may say it from — only namespaces carrying
`home-lab.example/redis-access: "true"`, the same shape the Gateway uses to decide
which namespaces may attach a route.

Its first intended caller is the admin panel's token refresh, which is what would
let `admin-web` run more than one replica. **That is not wired up yet**: the panel
still deduplicates refreshes in a process-local map, and its chart still pins one
replica for that reason. So Redis runs with nothing talking to it until that
lands — which is why the NetworkPolicy admits no namespace until one is
labelled.

## Storage

There is one StorageClass, `nfs-client`, it is the cluster default, and
**deleting a claim no longer destroys its data**. The provisioner renames the
directory on the export to `archived-<name>` instead of removing it.

**`reclaimPolicy` stays `Delete`, and that is not a contradiction.** It does not
say whether the data is deleted; it selects which handler runs when the claim
goes, and only `Delete` hands the volume to this provisioner. `Retain` would leave
the PV `Released` and never call the provisioner at all — so `archiveOnDelete`
would be read by nobody and nothing would ever be renamed. `Delete` is what makes
the archiving run. It is also the provisioner chart's own default, for the same
reason.

What changed is `archiveOnDelete`, from `false` to `true`, and the removal of an
`onDelete: delete` that overrode it. Changed globally rather than by adding a
second class for the one volume that needed it — the admin agent's memory, which
is months of conversations. Two classes is a choice at every future PVC, and the
day somebody forgets to make it is the day it mattered.

**The cost is that nothing reclaims space.** Every deleted claim leaves an
`archived-` directory until somebody removes it. That is a standing chore rather
than a solved problem; the prefix is what makes it scriptable, since orphaned data
is exactly the set matching `archived-*` and nothing live is ever named that
way.

## Configure and install

```bash
cp charts/platform/values.local.yaml.example \
  charts/platform/values.local.yaml
```

Replace the example domain, email, and NFS address in the ignored copy. Then:

```bash
./scripts/install-platform.sh \
  --values charts/platform/values.local.yaml
```

The installer performs two upgrades of the same release. The first starts
cert-manager after Gateway API CRDs exist; the second creates ClusterIssuers
and the optional Gateway certificate after the webhook is ready.

## Configure the remotely managed tunnel

In Cloudflare Zero Trust, add a public hostname for the enabled application.
For `whoami.example.com`, configure:

```text
Service:           HTTPS
Origin URL:        traefik.platform-system.svc.cluster.local:443
Origin Server Name: whoami.example.com
No TLS Verify:     disabled
```

Cloudflare terminates public TLS and verifies the Let's Encrypt certificate
presented by Traefik on the in-cluster hop. Protect administrative or private
hostnames with Cloudflare Access before leaving them enabled.

## Validate and remove the smoke test

```bash
PLATFORM_TEST_URL=https://whoami.example.com \
  ./scripts/validate-platform.sh
```

After the route works, disable `platform.whoami.enabled`, remove its Cloudflare
public hostname, and rerun the installer. The Traefik dashboard remains
disabled throughout.

## Uninstall and recovery

```bash
helm uninstall home-lab-platform --namespace platform-system
```

Uninstalling does not remove the Gateway API CRDs, retained cert-manager CRDs,
external Secrets, Cloudflare tunnel, DNS routes, or NFS data. Removing a PVC is
recoverable rather than destructive: the provisioner renames the directory to
`archived-<name>` instead of deleting it. The cost is that nothing reclaims that
space — the export grows until the archived directories are removed by hand.
