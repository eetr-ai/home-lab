# Libvirt host bridge prerequisite

Bridge networking places each virtual machine directly on the LAN. The router
provides DHCP, while stable VM MAC addresses map to operator-managed DHCP
reservations. Terraform attaches domains to an existing bridge; it deliberately
does not modify the host's active network configuration over SSH.

Keep the real interface name, MAC address, LAN subnet, host address, node
addresses, and router configuration in private operations notes. Replace every
placeholder below before running a command.

## Safety prerequisites

- Prove that the physical Ethernet interface receives an IPv4 DHCP lease.
- Keep a local console or a separately tested management connection available.
- Back up `/etc/netplan` before editing it.
- Reserve the host address in the router using the physical Ethernet MAC.
- Do not disable the recovery connection until the bridge is independently
  reachable.

## Configure the bridge

Merge this structure into the host's existing Netplan configuration. Do not
replace unrelated Wi-Fi or management configuration. `eno1` is an example
physical interface name, and the MAC is intentionally invalid:

```yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    eno1:
      dhcp4: false
      dhcp6: false
      accept-ra: false
      link-local: []
  bridges:
    br0:
      interfaces:
        - eno1
      macaddress: 00:00:00:00:00:00
      dhcp4: true
      dhcp6: true
      accept-ra: true
      dhcp4-overrides:
        route-metric: 100
      parameters:
        stp: false
        forward-delay: 0
```

Giving `br0` the physical interface MAC preserves the router's host reservation.
The physical port carries frames only; all host IP configuration belongs on the
bridge.

Validate syntax and apply with automatic rollback:

```bash
sudo netplan generate
sudo netplan try --timeout 120
```

Confirm the new connection from a separate terminal before accepting the
configuration. Then verify:

```bash
ip -brief link show br0
ip -4 -brief address show br0
ip -4 -brief address show eno1
ip route
bridge link
```

`br0` must hold the reserved host address and preferred default route. `eno1`
must be enslaved to `br0` and have no IP address of its own.

## Reserve node addresses

Create one router reservation for every VM MAC from the ignored
`terraform.tfvars`. Record the corresponding address as that node's
`ipv4_address`. Reservations must be unique and must not collide with other
static assignments.

The router, not Terraform, owns DHCP. Terraform records the expected mapping so
the Ansible inventory and kubeadm control-plane endpoint remain deterministic.

## Verify libvirt attachment

After `terraform apply`, verify each domain uses the host bridge:

```bash
virsh -c qemu:///system domiflist k8s-cp-1
virsh -c qemu:///system domiflist k8s-worker-1
virsh -c qemu:///system domiflist k8s-worker-2
```

The source must be `br0`, and each MAC must match both the ignored Terraform
values and its router reservation. The default libvirt NAT network may remain
installed for unrelated disposable workloads; the Kubernetes domains do not
use it.

## Recovery

If connectivity fails during `netplan try`, do not confirm the change; allow
the timer to restore the prior configuration. With console access, restore the
Netplan backup, run `sudo netplan generate`, and apply it. Do not proceed to VM
creation until the host bridge survives a reboot and remains reachable at its
reserved address.
