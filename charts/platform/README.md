# Home lab platform chart

This umbrella chart installs the services that make the kubeadm cluster useful
for applications:

- Traefik `v3.7.11` through chart `41.3.0`.
- cert-manager `v1.21.1` with Cloudflare DNS-01 issuers.
- NFS Subdirectory External Provisioner chart `4.0.18`.
- cloudflared `2026.8.2` for a remotely managed tunnel.
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

These Secrets are not owned by Helm. Helm values contain only their names and
keys.

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
external Secrets, Cloudflare tunnel, DNS routes, or NFS data unrelated to PVC
deletion. Removing a PVC is destructive because `nfs-client` uses the `Delete`
reclaim policy.
