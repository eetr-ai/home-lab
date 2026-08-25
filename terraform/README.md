# Terraform libvirt cluster

This stack provisions the three Ubuntu Server virtual machines that will form
the home-lab Kubernetes cluster. It runs on the operator's laptop and connects
to a remote libvirt host through OpenSSH.

Terraform manages:

- A directory-backed libvirt storage pool at a configurable host path.
- One released Ubuntu Server 26.04 LTS base image.
- One copy-on-write QCOW2 disk per node.
- One generated and uploaded cloud-init seed per node.
- Stable virtual MAC addresses and expected router-reserved IPv4 addresses.
- CPU, memory, UEFI, disk, guest-agent, and network domain configuration.

Cloud-init creates the administrator, installs base packages, enables the QEMU
guest agent, disables swap, and applies Kubernetes kernel prerequisites. This
stack intentionally does not use Terraform provisioners to run `kubeadm`.
Containerd and Kubernetes bootstrap will be managed by the Ansible layer.

## Node plan

The stack creates one control-plane node and two workers. CPU, memory, disk,
stable MAC addresses, and router-reserved IPv4 addresses are inputs in the
ignored `terraform.tfvars`; the tracked example contains documentation-only
values.

## Prerequisites

- Terraform `>= 1.10, < 2.0` on the laptop.
- Passwordless public-key SSH access from the laptop to the libvirt host.
- QEMU/KVM and libvirt running on the host.
- The SSH user on the host belongs to `libvirt` and `kvm`.
- The selected public key file exists on the laptop.
- Any manually created domains with the same node names have been removed.
- The host bridge exists and has working LAN connectivity; follow the
  [generic bridge guide](../docs/network-bridge.md).
- The router reserves every node address for the matching stable virtual MAC.

Verify remote libvirt access before applying:

```bash
ssh LIBVIRT_HOST virsh -c qemu:///system list --all
```

## Configure

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
```

Review the ignored `terraform.tfvars` and replace every placeholder. Use
`network_mode = "bridge"`, name the existing host bridge, and record each
router-reserved node address beside its stable MAC. Terraform carries those
addresses into downstream inventory but does not configure the router or
assign guest addresses itself. Never configure a private-key path.

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
ssh LIBVIRT_HOST virsh -c qemu:///system domiflist k8s-cp-1
ssh VM_ADMIN_USER@CONTROL_PLANE_RESERVED_ADDRESS
```

The VM receives its address from the LAN DHCP server. Terraform records the
expected reservation; verify the observed address in the router before running
Ansible.

## Optional temporary NAT mode

The stack retains NAT support for disposable tests:

```hcl
network_mode         = "network"
libvirt_network_name = "default"
```

NAT addresses come from libvirt rather than the router and require ProxyJump
through the host. They are unsuitable for the final directly reachable
control-plane endpoint.

## Change network attachment

Changing an existing cluster between NAT and bridge networking replaces its
domain network attachment and changes node identity assumptions. Treat it as a
rebuild: preserve required external keys and data, destroy the disposable VM
stack, change the ignored values, and apply again. For bridge mode:

```hcl
network_mode = "bridge"
bridge_name  = "br0"
```

Run a new plan and review it carefully. The host bridge is an operating-system
prerequisite and intentionally remains outside Terraform's control.

## State and secrets

Terraform state may contain infrastructure metadata and must not be committed.
The repository ignores local state and variable files. For a single operator,
keep state on the encrypted laptop and back it up. Before adding CI or a second
operator, migrate to a remote backend with encryption and locking.

Do not place passwords, private keys, kubeconfigs, tokens, or unencrypted
Kubernetes secrets in Terraform configuration or variable files.
