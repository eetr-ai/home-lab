variable "libvirt_uri" {
  description = "Libvirt connection URI. sshcmd uses the laptop's OpenSSH configuration."
  type        = string
}

variable "cluster_name" {
  description = "Prefix used for libvirt resources."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]*$", var.cluster_name))
    error_message = "cluster_name must contain lowercase letters, numbers, and hyphens."
  }
}

variable "storage_pool_path" {
  description = "Directory on the libvirt host used by the Terraform-managed pool."
  type        = string

  validation {
    condition     = startswith(var.storage_pool_path, "/")
    error_message = "storage_pool_path must be an absolute path on the libvirt host."
  }
}

variable "ubuntu_image_url" {
  description = "Released Ubuntu Server cloud image used as the immutable base volume."
  type        = string

  validation {
    condition     = startswith(var.ubuntu_image_url, "https://cloud-images.ubuntu.com/")
    error_message = "ubuntu_image_url must reference the official Ubuntu cloud image service."
  }
}

variable "ssh_public_key_path" {
  description = "Public key installed in each node. Private keys must never be supplied."
  type        = string

  validation {
    condition     = endswith(var.ssh_public_key_path, ".pub")
    error_message = "ssh_public_key_path must point to a .pub file."
  }
}

variable "admin_user" {
  description = "SSH and sudo user created by cloud-init."
  type        = string
}

variable "timezone" {
  description = "Timezone configured inside the nodes."
  type        = string
}

variable "upgrade_packages" {
  description = "Upgrade all Ubuntu packages during the first cloud-init boot."
  type        = bool
}

variable "cloud_init_revision" {
  description = "Increment to give rebuilt nodes a new cloud-init instance identity."
  type        = number
}

variable "network_mode" {
  description = "Use network for a libvirt virtual network such as default NAT, or bridge for host br0."
  type        = string

  validation {
    condition     = contains(["network", "bridge"], var.network_mode)
    error_message = "network_mode must be network or bridge."
  }
}

variable "libvirt_network_name" {
  description = "Existing libvirt virtual network used when network_mode is network."
  type        = string
}

variable "bridge_name" {
  description = "Existing host bridge used when network_mode is bridge."
  type        = string
}

variable "nodes" {
  description = "Kubernetes node sizing, stable virtual MAC addresses, and router-reserved IPv4 addresses."
  type = map(object({
    role         = string
    vcpu         = number
    memory_mib   = number
    disk_gib     = number
    mac          = string
    ipv4_address = optional(string)
  }))

  validation {
    condition = alltrue([
      for node in values(var.nodes) :
      contains(["control-plane", "worker"], node.role) &&
      node.vcpu >= 2 &&
      node.memory_mib >= 2048 &&
      node.disk_gib >= 20 &&
      can(regex("^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$", node.mac)) &&
      (node.ipv4_address == null ? true : can(cidrnetmask("${node.ipv4_address}/32")))
    ])
    error_message = "Each node needs a valid role, at least 2 vCPU, 2 GiB RAM, 20 GiB disk, a valid MAC, and an optional IPv4 address."
  }

  validation {
    condition = var.network_mode != "bridge" || alltrue([
      for node in values(var.nodes) : node.ipv4_address != null
    ])
    error_message = "Every node must declare its router-reserved ipv4_address when network_mode is bridge."
  }

  validation {
    condition = length([
      for node in values(var.nodes) : node.ipv4_address
      if node.ipv4_address != null
      ]) == length(distinct([
        for node in values(var.nodes) : node.ipv4_address
        if node.ipv4_address != null
    ]))
    error_message = "Node ipv4_address values must be unique."
  }
}
