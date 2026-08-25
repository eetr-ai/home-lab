output "nodes" {
  description = "Provisioned node identities. Addresses are assigned by the selected DHCP server."
  value = {
    for name, node in var.nodes : name => {
      role        = node.role
      mac         = node.mac
      domain_uuid = libvirt_domain.node[name].uuid
    }
  }
}

output "libvirt_uri" {
  description = "Libvirt endpoint used by this stack."
  value       = var.libvirt_uri
}

output "network_attachment" {
  description = "Selected VM network attachment."
  value = var.network_mode == "bridge" ? {
    mode   = "bridge"
    source = var.bridge_name
    } : {
    mode   = "network"
    source = var.libvirt_network_name
  }
}

output "storage_pool" {
  description = "Terraform-managed libvirt storage pool."
  value = {
    name = libvirt_pool.k8s.name
    path = var.storage_pool_path
  }
}
