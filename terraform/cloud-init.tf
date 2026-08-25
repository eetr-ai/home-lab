resource "libvirt_cloudinit_disk" "node" {
  for_each = var.nodes

  name = "${each.key}-cloud-init"

  meta_data = yamlencode({
    instance-id    = "${var.cluster_name}-${each.key}-${var.cloud_init_revision}"
    local-hostname = each.key
  })

  user_data = templatefile("${path.module}/templates/user-data.yaml.tftpl", {
    admin_user       = var.admin_user
    hostname         = each.key
    node_role        = each.value.role
    ssh_public_key   = local.ssh_public_key
    timezone         = var.timezone
    upgrade_packages = var.upgrade_packages
  })
}

resource "libvirt_volume" "cloud_init" {
  for_each = var.nodes

  name = "${each.key}-cloud-init.iso"
  pool = libvirt_pool.k8s.name

  target = {
    format = {
      type = "iso"
    }
  }

  create = {
    content = {
      url = libvirt_cloudinit_disk.node[each.key].path
    }
  }

  lifecycle {
    replace_triggered_by = [libvirt_cloudinit_disk.node[each.key]]
  }
}
