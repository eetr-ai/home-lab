# Home Lab Infrastructure

Infrastructure-as-code and operational documentation for a home-lab server.

The repository turns an Ubuntu Server host into a repeatable virtualization
and Kubernetes environment. The physical server is the foundation, virtual
machines provide the Kubernetes nodes, and the cluster supplies application
routing, storage provisioning, certificates, and selected public services
through Cloudflare Tunnel.

## Current host

The physical inventory, network addresses, storage device names, and exact
partition sizes are kept in the private operations document. Public examples
in this repository use placeholders; real values belong in ignored variable
files.

## Target architecture

```text
Internet
   |
Cloudflare Tunnel
   |
Traefik Gateway API
   |
Ubuntu Server virtualization host
   |
   +-- Linux bridge: br0
   |   +-- k8s-cp-1      control plane
   |   +-- k8s-worker-1  worker
   |   +-- k8s-worker-2  worker
   |
   +-- QEMU/KVM and libvirt
   +-- NFS export for Kubernetes volumes
   +-- Docker-hosted PostgreSQL and MongoDB
   +-- LVM-backed host storage
```

The implemented stack includes:

- QEMU/KVM virtual machines managed through libvirt.
- A three-node Kubernetes cluster bootstrapped with `kubeadm`.
- Cloud-init for repeatable VM initialization.
- Traefik with Kubernetes Gateway API for application routing.
- cert-manager with Cloudflare DNS-01 for managed TLS certificates.
- NFS-backed dynamic volume provisioning.
- Cloudflare Tunnel for explicitly approved public endpoints.
- Host-managed PostgreSQL with pgvector and MongoDB for private workloads.
- Terraform for VM infrastructure.
- Ansible for Kubernetes-node configuration and kubeadm bootstrap.
- Pinned upstream Helm charts and a repository-owned platform chart.

The delivery boundary is deliberately explicit:

```text
terraform apply
    -> ansible-playbook
        -> helm upgrade --install
```

Terraform defines the Ubuntu VMs and cloud-init configuration. Ansible
configures the operating systems and bootstraps kubeadm. The standalone Cilium
release supplies cluster networking. The platform chart installs Traefik,
cert-manager, NFS provisioning, and cloudflared.

## Consolidated setup guide

This is the shortest complete build path. Follow the linked component guide
when a step needs troubleshooting or recovery detail. Commands run on the
operator laptop unless they explicitly say otherwise.

### 1. Prepare the host and LAN

On the Ubuntu Server virtualization host:

1. Install QEMU/KVM, libvirt, and NFS server packages.
2. Mount VM image storage and the NFS export outside the root filesystem.
3. Configure the existing Ethernet interface as a member of `br0` using the
   [bridge guide](docs/network-bridge.md). Use `netplan try`, retain console
   recovery, and prove the bridge survives a reboot.
4. Reserve the host address in the router against the physical Ethernet MAC.
5. Reserve one unique LAN address for each stable VM MAC in the ignored
   Terraform values.
6. Export the Kubernetes NFS directory only to the selected node addresses,
   following the [NFS guide](docs/nfs-server.md).

Verify the host before creating VMs:

```bash
ip -4 -brief address show br0
ip -4 -brief address show PHYSICAL_INTERFACE
ip route
virsh -c qemu:///system list --all
findmnt /srv/images
findmnt /srv/nfs/k8s
sudo exportfs -v
```

`br0` must own the host LAN address and default route. The physical interface
must have no IP address of its own. Keep real interface names, addresses,
storage devices, and router configuration in private operations notes.

### 2. Configure operator inputs

Install Terraform `>= 1.10, < 2.0`, Ansible Core, Helm, the Cilium CLI, `jq`,
and OpenSSH on the laptop. Confirm public-key SSH access to the libvirt host.

Create the ignored Terraform values:

```bash
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
```

Replace every placeholder. The final topology uses:

```hcl
network_mode = "bridge"
bridge_name  = "br0"
```

Each node must declare the stable MAC reserved in the router and its matching
`ipv4_address`. Supply only a public-key path; never configure private-key
material in Terraform.

### 3. Create the virtual machines

```bash
terraform -chdir=terraform init
terraform fmt -check -recursive
terraform -chdir=terraform validate
terraform -chdir=terraform plan -out=cluster.tfplan
terraform -chdir=terraform apply cluster.tfplan
```

Review the plan before applying. A `cloud_init_revision` change is disruptive
because it replaces each affected cloud-init volume and domain.

Confirm the domains use the bridge and the reserved addresses are reachable:

```bash
ssh LIBVIRT_HOST virsh -c qemu:///system domiflist k8s-cp-1
ssh LIBVIRT_HOST virsh -c qemu:///system domiflist k8s-worker-1
ssh LIBVIRT_HOST virsh -c qemu:///system domiflist k8s-worker-2
ssh VM_ADMIN_USER@CONTROL_PLANE_RESERVED_ADDRESS
```

See the [Terraform guide](terraform/README.md) for storage, NAT fallback, and
network-change behavior.

### 4. Generate inventory and protect the encryption key

Render the ignored direct-LAN Ansible inventory from Terraform output:

```bash
./scripts/render-ansible-inventory.sh \
  --vm-user YOUR_VM_ADMIN_USER

cd ansible
ansible kubernetes -m ansible.builtin.ping
cd ..
```

Create exactly 32 random bytes in a mode-`0600` file outside this repository:

```bash
KEY_PATH=/ABSOLUTE/SECURE/PATH/kubernetes-secretbox.key
if [ -e "$KEY_PATH" ]; then
  printf '%s\n' "Refusing to overwrite existing key: $KEY_PATH" >&2
  exit 1
fi

umask 077
(set -C; openssl rand 32 >"$KEY_PATH") || exit 1
chmod 0600 "$KEY_PATH"
test "$(wc -c <"$KEY_PATH" | tr -d '[:space:]')" -eq 32
```

Store a recoverable copy somewhere independent of the cluster before
initialization. Losing this key makes encrypted Kubernetes Secrets in etcd
unrecoverable. The existence check and shell no-clobber mode deliberately make
this command fail rather than replace an established key. Never print, paste,
or commit it.

### 5. Bootstrap Kubernetes

```bash
cd ansible
ansible-playbook playbooks/bootstrap.yml \
  -e encryption_key_file=/ABSOLUTE/SECURE/PATH/kubernetes-secretbox.key \
  -e encryption_key_backup_confirmed=true
cd ..
```

The playbook installs pinned Kubernetes v1.36.4 packages and containerd,
initializes the control plane only when required, joins unconfigured workers,
and fetches the administrator kubeconfig to the ignored
`ansible/artifacts/admin.conf`. It also installs `kubectl` on every Kubernetes
VM and makes `$HOME/.kube/config` the default for the VM administrator on the
control plane.

Run the same playbook a second time. It must not reinitialize the control plane
or rejoin workers. See the [Ansible guide](ansible/README.md) for its security
invariants and recovery boundary.

### 6. Install Cilium and validate the cluster

```bash
export KUBECONFIG="$PWD/ansible/artifacts/admin.conf"

helm upgrade --install cilium oci://quay.io/cilium/charts/cilium \
  --version 1.20.1 \
  --namespace kube-system \
  --values helm-values/cilium.yaml \
  --wait

./scripts/validate-cluster.sh
```

The operator laptop remains the primary administration endpoint. To install
the pinned client and make this kubeconfig the default on the physical
virtualization host as well, follow the optional
[server kubectl guide](docs/server-kubectl.md).

The validation requires three Ready v1.36.4 nodes, waits for Cilium, runs its
connectivity test, and proves a test Secret is stored in etcd through the
`secretbox` encryption envelope without plaintext.

### 7. Create platform credentials

Create the Cloudflare DNS API token and remotely managed tunnel token as
separate mode-`0600` files outside the repository. The DNS token must be scoped
to `Zone:DNS:Edit` and `Zone:Zone:Read` for only the intended zone.

Create the namespace and external Secrets before installing the chart:

```bash
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

These Secrets are intentionally not owned by Helm.

### 8. Install and validate the platform

```bash
cp charts/platform/values.local.yaml.example \
  charts/platform/values.local.yaml
```

Replace the example domain, email, NFS server address, and NFS path in the
ignored values file. Then install:

```bash
./scripts/install-platform.sh \
  --values charts/platform/values.local.yaml
```

In Cloudflare Zero Trust, route each approved public hostname to:

```text
Service:            HTTPS
Origin URL:         traefik.platform-system.svc.cluster.local:443
Origin Server Name: the same public hostname
No TLS Verify:      disabled
```

Enable the optional whoami route while proving the full path, then run:

```bash
PLATFORM_TEST_URL=https://YOUR_TEST_HOSTNAME \
  ./scripts/validate-platform.sh
```

After validation, disable whoami, remove its Cloudflare public hostname, and
rerun the platform installer. The [platform guide](charts/platform/README.md)
documents Gateway API routing, certificates, uninstall behavior, and PVC data
risk.

### 9. Install private host databases when needed

The optional [database module](databases/README.md) runs PostgreSQL with
pgvector and MongoDB directly on the virtualization host. Two Docker Compose
projects keep the containers running, store data on the dedicated
`/srv/datastore` volume, and publish the native database ports so the operator
laptop, the host, and the Kubernetes nodes all connect the same way.

Setup is an ignored environment file with two superuser passwords, one per
server. Compose drives the host through a Docker context over SSH, so the
module runs from the laptop checkout without copying anything to the server.
Those credentials are what the planned in-cluster infra admin application uses
to create and manage databases; the module itself creates no application
database. Database ports never receive a Cloudflare route, and the password is
the access boundary.

### 10. Final checks

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl get gateway,httproute -A
kubectl get storageclass,pv,pvc -A
cilium status --wait
```

Confirm that:

- the kubeconfig reaches the control-plane LAN address directly;
- no SSH API tunnel or ProxyJump remains in the bridge topology;
- NFS-backed PVC creation and deletion behavior is understood;
- administrative routes are protected by Cloudflare Access;
- databases are not exposed publicly; and
- no key, token, kubeconfig, state, or unencrypted Secret appears in Git, CI,
  Terraform state, Ansible logs, or Helm values.

## Releasing the admin panel

The [admin panel](admin/) is versioned independently of the infrastructure around
it. release-please reads the Conventional Commit history and keeps a release pull
request open; merging it tags `admin-vX.Y.Z`. That tag fires the Cloud Build
trigger declared in [terraform/gcp](terraform/gcp/README.md), which builds the
images and publishes them to Artifact Registry under the tag's version.

Commits touching only `ansible/`, `terraform/`, or `charts/platform/` are outside
the released package, so infrastructure work never moves the panel's version.

To exercise the pipeline without cutting a release:

```bash
gcloud builds submit --config cloudbuild.yaml \
  --substitutions=_IMAGE_BASE=us-west1-docker.pkg.dev/YOUR_PROJECT/home-lab,_TAG=dev-local \
  --service-account=projects/YOUR_PROJECT/serviceAccounts/home-lab-build@YOUR_PROJECT.iam.gserviceaccount.com \
  .
```

Installing a published version into the cluster stays a deliberate step, not
something a tag does on its own:

```bash
export KUBECONFIG=ansible/artifacts/admin.conf
task admin:install -- --values charts/admin/values.local.yaml
```

The [admin chart guide](charts/admin/README.md) covers the values to replace and
what the release does and does not own. The nodes already hold the registry
credential, so there is no per-namespace pull secret to create first — see the
[Ansible guide](ansible/README.md).

## Rebuild and recovery boundaries

- Preserve the secretbox key and durable NFS data before replacing nodes.
- A router reservation or control-plane address change affects kubeadm API
  identity and must be treated as a deliberate rebuild or recovery.
- Reusing a reserved address after recreating a VM changes its SSH host key.
  Remove only the confirmed stale entry with `ssh-keygen -R ADDRESS`.
- Deleting an NFS-backed PVC is destructive because the default StorageClass
  uses the `Delete` reclaim policy.
- Terraform state, generated inventory, fetched kubeconfig, local Helm values,
  and all credential files must remain ignored and backed up appropriately.

## Repository layout

```text
.
├── admin/            # The in-cluster administration panel
│   └── api/          # Go API server managing the databases and reading the cluster
├── ansible/          # Kubernetes-node configuration and kubeadm bootstrap
├── docs/             # Architecture, operations, and recovery guides
├── charts/           # Cluster platform and repository-owned Helm charts
├── helm-values/      # Reviewed values for standalone upstream charts
├── cloud-init/       # Cloud-init examples and learning artifacts
├── databases/        # Private host PostgreSQL and MongoDB services
├── scripts/          # Small, idempotent operational helpers
├── terraform/        # libvirt VMs, and the Google Cloud build and registry
└── tests/            # Repository regression fixtures and checks
```

## Working in this repository

Changes to `main` are made through pull requests. A pull request must pass the
repository checks and resolve all review conversations before it can be
merged. See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.

Checks are defined in [Taskfile.yml](Taskfile.yml) and run through
[go-task](https://taskfile.dev), so the command that gates a pull request is the
same one you run locally:

```bash
task check
```

`task --list` shows the rest, including the operational entry points. The
conventions the repository follows — commits, layering, testing, and the admin
panel's interface rules — are in [docs/contributing/](docs/contributing/README.md),
which is also what coding agents read.

This is a public repository. Never commit private keys, passwords, API tokens,
kubeconfig files, Terraform state, Cloudflare tunnel credentials, database
credentials, or unencrypted Kubernetes secrets.

Environment-specific inventory and the decision log remain in the private Home
Infra Notion page. This README and the linked component guides are the public,
reproducible setup source of truth.
