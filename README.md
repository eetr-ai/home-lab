# Home Lab Infrastructure

Infrastructure-as-code and operational documentation for a home-lab server.

The repository will turn a freshly installed Ubuntu Server host into a
repeatable virtualization and Kubernetes environment. The physical server is
the foundation; virtual machines provide the Kubernetes nodes; and the
cluster hosts applications, databases, ingress, storage provisioning, and
selected public services through Cloudflare Tunnel.

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
Kubernetes ingress
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
   +-- LVM-backed host storage
```

The planned platform includes:

- QEMU/KVM virtual machines managed through libvirt.
- A three-node Kubernetes cluster bootstrapped with `kubeadm`.
- Cloud-init for repeatable VM initialization.
- NFS-backed dynamic Kubernetes volume provisioning.
- MetalLB and NGINX Ingress for services on the local network.
- PostgreSQL and MongoDB for lab projects.
- Cloudflare Tunnel for explicitly approved public endpoints.
- Terraform/OpenTofu for VM infrastructure and cluster integrations.
- Ansible or idempotent scripts for host and Kubernetes-node configuration.
- Helm or GitOps for services running inside the cluster.

## Delivery phases

1. Document and prepare the Ubuntu host, networking, LVM, and NFS.
2. Install and validate QEMU/KVM and the `br0` network bridge.
3. Define the three virtual machines with Terraform/OpenTofu and cloud-init.
4. Configure the nodes and bootstrap Kubernetes with `kubeadm`.
5. Install storage, load balancing, ingress, databases, and Cloudflare Tunnel.
6. Add observability, backups, secret management, and GitOps.

## Planned repository layout

```text
.
├── ansible/          # Host and Kubernetes-node configuration
├── docs/             # Architecture, operations, and recovery guides
├── helm-values/      # Reviewed values for installed Helm charts
├── k8s/              # Kubernetes resources and GitOps definitions
├── scripts/          # Small, idempotent operational helpers
└── terraform/        # libvirt VMs and infrastructure integrations
```

Directories will be introduced as their corresponding implementation phase
begins. The repository should describe the infrastructure that actually
exists, not only the intended end state.

## Working in this repository

Changes to `main` are made through pull requests. A pull request must pass the
repository checks and resolve all review conversations before it can be
merged. See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.

This is a public repository. Never commit private keys, passwords, API tokens,
kubeconfig files, Terraform state, Cloudflare tunnel credentials, database
credentials, or unencrypted Kubernetes secrets.

## Documentation

The detailed build guide and decision log currently live in the private Home
Infra Notion page. Durable configuration, runbooks, and recovery procedures
will move into `docs/` as they are implemented so that the repository remains
the reproducible source of truth.
