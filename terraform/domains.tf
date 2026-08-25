resource "libvirt_domain" "node" {
  for_each = var.nodes

  name        = each.key
  title       = "${var.cluster_name} ${each.value.role}"
  description = "Terraform-managed Kubernetes ${each.value.role} node"
  type        = "kvm"
  memory      = each.value.memory_mib
  memory_unit = "MiB"
  vcpu        = each.value.vcpu
  autostart   = true
  running     = true

  cpu = {
    mode = "host-passthrough"
  }

  features = {
    acpi = true
  }

  os = {
    type         = "hvm"
    type_arch    = "x86_64"
    type_machine = "q35"
    firmware     = "efi"
    boot_devices = [{ dev = "hd" }]
  }

  devices = {
    disks = [
      {
        device = "disk"
        driver = {
          name    = "qemu"
          type    = "qcow2"
          discard = "unmap"
        }
        source = {
          file = {
            file = libvirt_volume.node_disk[each.key].path
          }
        }
        target = {
          dev = "vda"
          bus = "virtio"
        }
      },
      {
        device    = "cdrom"
        read_only = true
        driver = {
          name = "qemu"
          type = "raw"
        }
        source = {
          file = {
            file = libvirt_volume.cloud_init[each.key].path
          }
        }
        target = {
          dev = "sda"
          bus = "sata"
        }
      }
    ]

    interfaces = [
      {
        mac = {
          address = each.value.mac
        }
        model = {
          type = "virtio"
        }
        source = {
          network = var.network_mode == "network" ? {
            network = var.libvirt_network_name
          } : null
          bridge = var.network_mode == "bridge" ? {
            bridge = var.bridge_name
          } : null
        }
      }
    ]

    channels = [
      {
        source = {
          unix = {
            mode = "bind"
          }
        }
        target = {
          virt_io = {
            name = "org.qemu.guest_agent.0"
          }
        }
      }
    ]

  }

  lifecycle {
    replace_triggered_by = [libvirt_volume.cloud_init[each.key]]
  }

  depends_on = [
    libvirt_volume.node_disk,
    libvirt_volume.cloud_init,
  ]
}
