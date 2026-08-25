resource "libvirt_pool" "k8s" {
  name = local.pool_name
  type = "dir"

  target = {
    path = var.storage_pool_path
  }
}

resource "libvirt_volume" "ubuntu_base" {
  name = "ubuntu-26.04-server-cloudimg-amd64.qcow2"
  pool = libvirt_pool.k8s.name

  target = {
    format = {
      type = "qcow2"
    }
  }

  create = {
    content = {
      url = var.ubuntu_image_url
    }
  }
}

resource "libvirt_volume" "node_disk" {
  for_each = var.nodes

  name     = "${each.key}.qcow2"
  pool     = libvirt_pool.k8s.name
  capacity = each.value.disk_gib * 1024 * 1024 * 1024

  target = {
    format = {
      type = "qcow2"
    }
  }

  backing_store = {
    path = libvirt_volume.ubuntu_base.path
    format = {
      type = "qcow2"
    }
  }
}
