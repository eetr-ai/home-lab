# Terraform libvirt cluster

This stack provisions the three Ubuntu Server virtual machines that will form
the home-lab Kubernetes cluster. It runs on the operator's laptop and connects
to a remote libvirt host through OpenSSH.

Terraform manages:

- A directory-backed libvirt storage pool at a configurable host path.
- One released Ubuntu Server 26.04 LTS base image.
- One copy-on-write QCOW2 disk per node.
- One generated and uploaded cloud-init seed per node.
- Stable virtual MAC addresses.
- CPU, memory, UEFI, disk, guest-agent, and network domain configuration.

Cloud-init creates the administrator, installs base packages, enables the QEMU
guest agent, disables swap, and applies Kubernetes kernel prerequisites. This
stack intentionally does not use Terraform provisioners to run `kubeadm`.
Containerd and Kubernetes bootstrap will be managed by the Ansible layer.

## Node plan

The stack creates one control-plane node and two workers. CPU, memory, disk,
and stable MAC addresses are required inputs in the ignored
`terraform.tfvars`; the tracked example contains non-production values.

## Prerequisites

- Terraform `>= 1.10, < 2.0` on the laptop.
- Passwordless public-key SSH access from the laptop to the libvirt host.
- QEMU/KVM and libvirt running on the host.
- The SSH user on the host belongs to `libvirt` and `kvm`.
- The selected public key file exists on the laptop.
- Any manually created domains with the same node names have been removed.

Verify remote libvirt access before applying:

```bash
ssh LIBVIRT_HOST virsh -c qemu:///system list --all
```

The current manual `k8s-cp-1` proof of concept conflicts with the Terraform
domain name. Destroy it only when ready to replace it:

```bash
ssh LIBVIRT_HOST virsh -c qemu:///system destroy k8s-cp-1
ssh LIBVIRT_HOST virsh -c qemu:///system undefine k8s-cp-1 --nvram
```

Its manually created QCOW2 and cloud-init artifacts are outside the
Terraform-managed pool and can be retained until the Terraform deployment has
been validated.

## Configure

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
```

Every environment-specific variable is required. Review the ignored
`terraform.tfvars` and replace every placeholder. For the initial build, use
libvirt's NAT network. Never configure a private-key path.

Incrementing `cloud_init_revision` replaces each affected cloud-init volume and
domain so the VM reboots and cloud-init sees a new instance identity. Review
that plan as a disruptive node operation; it does not replace the node disk.

The public-key file is read from the laptop only while Terraform renders the
cloud-init seed. Key material is not stored in the repository; the real tfvars,
generated plans, state, and personalized cloud-init files are ignored.

## Deploy

```bash
terraform init
terraform fmt -check -recursive
terraform validate
terraform plan -out=cluster.tfplan
terraform apply cluster.tfplan
```

The initial apply downloads the Ubuntu base image and uploads it to the remote
libvirt pool, so it takes longer than subsequent operations.

Inspect the result:

```bash
ssh LIBVIRT_HOST virsh -c qemu:///system list --all
ssh LIBVIRT_HOST virsh -c qemu:///system net-dhcp-leases default
```

With NAT, reach a node through the libvirt host:

```bash
ssh -J LIBVIRT_HOST VM_ADMIN_USER@NODE_IP
```

## Migrate from NAT to Ethernet bridge

After `eno1` and host bridge `br0` are configured, change:

```hcl
network_mode = "bridge"
bridge_name  = "br0"
```

Run a new plan and review the interface replacements carefully. The stable MAC
addresses should be used for router DHCP reservations.

## State and secrets

Terraform state may contain infrastructure metadata and must not be committed.
The repository ignores local state and variable files. For a single operator,
keep state on the encrypted laptop and back it up. Before adding CI or a second
operator, migrate to a remote backend with encryption and locking.

Do not place passwords, private keys, kubeconfigs, tokens, or unencrypted
Kubernetes secrets in Terraform configuration or variable files.
