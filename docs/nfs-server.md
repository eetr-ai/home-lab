# NFS server prerequisite

The platform chart expects an existing NFSv4 export that is reachable from the
Kubernetes nodes. This service must exist before Helm installs the dynamic NFS
provisioner.

Keep the actual server address, VM subnet, interface name, filesystem layout,
and export path in private operations documentation and ignored local values.
The examples below use documentation-only placeholders.

Set these values for your environment before running the commands:

```bash
NFS_EXPORT_PATH=/export/kubernetes
VM_SUBNET_CIDR=192.0.2.0/24
VIRTUAL_BRIDGE=br0
```

## Install and export the filesystem

Run on the NFS server:

```bash
sudo apt update
sudo apt install -y nfs-kernel-server

sudo install -d -m 0755 /etc/exports.d
sudo install -d -o nobody -g nogroup -m 0770 "$NFS_EXPORT_PATH"
printf '%s %s(rw,sync,no_subtree_check,root_squash)\n' \
  "$NFS_EXPORT_PATH" "$VM_SUBNET_CIDR" \
  | sudo tee /etc/exports.d/kubernetes.exports >/dev/null

sudo exportfs -rav
sudo systemctl enable --now nfs-server
```

`root_squash` prevents root in a VM or pod from becoming host root through the
export. Limit the export to the Kubernetes-node subnet. Do not replace it with
`no_root_squash` or a wildcard client range.

If UFW is active, allow NFSv4 only from the node subnet:

```bash
sudo ufw allow in on "$VIRTUAL_BRIDGE" \
  from "$VM_SUBNET_CIDR" to any port 2049 proto tcp
```

Verify on the server:

```bash
findmnt "$NFS_EXPORT_PATH"
ip -brief address show "$VIRTUAL_BRIDGE"
sudo exportfs -v
systemctl is-active nfs-server
```

`sudo exportfs -rav` is sufficient after changing an export. Restarting the NFS
server does not repair a stale server address or client ACL.

## Verify from one Kubernetes node

Replace the documentation address and path with ignored local values:

```bash
NFS_SERVER_ADDRESS=192.0.2.1
NFS_EXPORT_PATH=/export/kubernetes

sudo install -d /mnt/nfs-prerequisite-check
sudo mount -t nfs4 -o nfsvers=4.1 \
  "$NFS_SERVER_ADDRESS:$NFS_EXPORT_PATH" \
  /mnt/nfs-prerequisite-check
mountpoint /mnt/nfs-prerequisite-check
sudo touch /mnt/nfs-prerequisite-check/.write-check
sudo rm /mnt/nfs-prerequisite-check/.write-check
sudo umount /mnt/nfs-prerequisite-check
```

The platform chart creates `nfs-client` as the default StorageClass. Its
reclaim policy is deliberately `Delete`, with archival disabled. Deleting a
PVC therefore deletes its provisioned directory and data. Back up important
data before deleting a claim.

## Migrate from libvirt NAT to the LAN bridge

The libvirt NAT gateway address is reachable only while the guests use that
private network. After moving the Kubernetes domains to `br0`, update both the
server export clients and the ignored platform values to use the bridged
addresses:

```yaml
nfs-provisioner:
  nfs:
    server: NFS_SERVER_LAN_ADDRESS
    path: /export/kubernetes
```

Replace the old NAT client range in `/etc/exports.d/kubernetes.exports` with
the reserved node LAN addresses (or their narrowly scoped subnet), then run:

```bash
sudo exportfs -rav
sudo exportfs -v
```

Prove TCP port 2049 and a read/write NFSv4.1 mount from a worker before changing
Kubernetes resources. A provisioner pod stuck in `ContainerCreating` usually
reports the underlying mount error in `kubectl describe pod` events.

The chart's bootstrap PV records the NFS server in an immutable field. During a
fresh installation with no provisioned application data, discard the failed
bootstrap PV and PVC after correcting the export and ignored Helm values:

```bash
kubectl -n platform-system scale \
  deployment/nfs-subdir-external-provisioner --replicas=0
kubectl -n platform-system wait --for=delete pod \
  --selector=app=nfs-provisioner --timeout=2m
kubectl -n platform-system delete \
  pvc/pvc-nfs-subdir-external-provisioner
kubectl delete pv/pv-nfs-subdir-external-provisioner
```

Rerun `scripts/install-platform.sh` to recreate them. Do not use this cleanup
as a general migration procedure after workloads contain durable data; plan and
back up that migration explicitly.

Direct LAN ingress is a separate topology decision. Do not infer a LAN address,
bridge, load balancer, or host-forwarding configuration from this generic
guide.
