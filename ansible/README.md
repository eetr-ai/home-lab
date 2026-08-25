# Ansible Kubernetes bootstrap

This layer turns the Terraform-created Ubuntu VMs into a kubeadm Kubernetes
cluster. It pins Kubernetes v1.36.4 (`1.36.4-1.1` Debian packages), configures
containerd with systemd cgroups, initializes one control plane with encrypted
Secrets at rest, and joins two workers using short-lived tokens.

Ansible does not install Cilium. The operational boundary remains:

```text
terraform apply
ansible-playbook
helm upgrade --install
```

## Security invariants

- The secretbox key is exactly 32 bytes, stored in a mode-`0600` file outside
  this repository, and never copied into Terraform state or Git.
- Bootstrap refuses to initialize until the operator confirms that a
  recoverable copy of the key exists.
- Key and join-token tasks use `no_log`; `kubeadm init` output is also hidden
  because it contains a bootstrap token.
- An initialized control plane refuses a different encryption key. Rotating a
  key requires a deliberate migration that retains the old decryption key
  until every stored object has been rewritten.
- The fetched administrator kubeconfig and generated inventory are ignored and
  mode `0600`. Both contain environment-specific or privileged data.
- SSH host-key checking stays enabled. When a disposable VM is rebuilt at a
  reused reserved address, remove only that stale entry with `ssh-keygen -R ADDRESS`
  after independently verifying that the VM was intentionally replaced.

Losing the encryption key makes encrypted Secrets in etcd unrecoverable. Store
the backup somewhere independent of the control-plane VM before initialization.

## Prerequisites

On the operator laptop, install Terraform, Ansible Core, Helm, the Cilium CLI,
`jq`, and OpenSSH. Terraform must already have applied the VM stack, and all
three VMs must be reachable at their router-reserved addresses.

For bridge mode, the generated inventory reads direct node addresses from
Terraform output and does not configure ProxyJump. Terraform does not create
the router reservations; its ignored input records the operator-managed
address contract.

## 1. Generate the ignored inventory

From the repository root, provide the VM administrator:

```bash
./scripts/render-ansible-inventory.sh \
  --vm-user YOUR_VM_ADMIN_USER
```

The script reads node names, roles, and reserved IPv4 addresses from Terraform
output and writes `ansible/inventory/hosts.yml` with mode `0600`.

For optional NAT mode, provide the libvirt host. The network defaults to the
source recorded by Terraform, but `--network` can override it:

```bash
./scripts/render-ansible-inventory.sh \
  --vm-user YOUR_VM_ADMIN_USER \
  --libvirt-host YOUR_LIBVIRT_HOST
```

Review connectivity before making changes:

```bash
cd ansible
ansible kubernetes -m ansible.builtin.ping
```

## 2. Create and safeguard the encryption key

Create exactly 32 random bytes outside the repository. The destination must be
an absolute path on storage that is backed up separately:

```bash
umask 077
openssl rand 32 > /ABSOLUTE/PATH/OUTSIDE/REPOSITORY/kubernetes-secretbox.key
stat -f '%Sp %z bytes' /ABSOLUTE/PATH/OUTSIDE/REPOSITORY/kubernetes-secretbox.key
```

On Linux, use `stat -c '%A %s bytes'` for the final check. Store a recoverable
copy now. Do not encode, print, paste, or commit the key.

## 3. Bootstrap the nodes

From `ansible/`, run:

```bash
ansible-playbook playbooks/bootstrap.yml \
  -e encryption_key_file=/ABSOLUTE/PATH/OUTSIDE/REPOSITORY/kubernetes-secretbox.key \
  -e encryption_key_backup_confirmed=true
```

The common role verifies the platform, swap, hostnames, modules, sysctls, and
package versions. It installs containerd, enables systemd cgroups, and installs
and holds kubelet, kubeadm, and kubectl. The control-plane role writes kubeadm
v1beta4 configuration, mounts the secretbox configuration in kube-apiserver,
and initializes only when `/etc/kubernetes/admin.conf` is absent. Each worker
joins only when `/etc/kubernetes/kubelet.conf` is absent.

The control-plane endpoint is the stable router-reserved `k8s-cp-1` address in
the generated inventory. kubeadm includes that address in the API server
certificate, and the fetched kubeconfig is directly usable from the LAN.

The playbook fetches the administrator kubeconfig to the ignored
`ansible/artifacts/admin.conf` file.

## 4. Install Cilium from its upstream OCI chart

Cilium is pinned to v1.20.1. kube-proxy stays enabled. Cluster-pool IPAM is
explicitly restricted to `10.244.0.0/16`; this avoids Cilium's conflicting
`10.0.0.0/8` default. From the repository root:

```bash
export KUBECONFIG="$PWD/ansible/artifacts/admin.conf"
helm upgrade --install cilium oci://quay.io/cilium/charts/cilium \
  --version 1.20.1 \
  --namespace kube-system \
  --values helm-values/cilium.yaml \
  --wait
```

## 5. Prove rerun safety and validate

Run the same Ansible command a second time. It must not initialize the control
plane, generate join tokens, or join workers again. Package-cache timestamps or
download checks may run, but no cluster identity should change.

Then run the validation from the repository root:

```bash
export KUBECONFIG="$PWD/ansible/artifacts/admin.conf"
./scripts/validate-cluster.sh
```

The script requires exactly three Ready nodes on v1.36.4, waits for Cilium,
runs the Cilium connectivity test, creates a temporary Secret, and checks its
raw etcd record. The record must use the `secretbox` envelope and must not
contain the plaintext marker. The temporary Secret and local record are removed
on exit.

## Helm boundary

Use pinned upstream charts plus tracked, non-secret values for Cilium and later
platform components. Do not create charts for kubeadm, containerd, encryption
configuration, namespaces, or wrappers around upstream Cilium. A custom chart
belongs here only when an application owned by this repository has a cohesive
set of configurable Kubernetes resources. Secrets never belong in Helm values
or Git.

Cilium remains a standalone bootstrap release because the cluster needs a CNI
before other services can run. After Cilium validates, install the platform
umbrella chart described in [the platform guide](../charts/platform/README.md).
Its Traefik routing
contract is Kubernetes Gateway API, not `Ingress` or Traefik-specific routes.

## Rebuild and recovery

A rerun is safe only when the cluster identity remains intact. Replacing a VM,
changing the control-plane address, rotating the encryption key, or running
`kubeadm reset` is an explicit recovery or rebuild operation. Back up the
secretbox key and any durable cluster data first. The ignored kubeconfig can be
re-fetched from `/etc/kubernetes/admin.conf` when the control plane is healthy.
