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

Direct LAN ingress is a separate topology decision. Do not infer a LAN address,
bridge, load balancer, or host-forwarding configuration from this generic
guide.
