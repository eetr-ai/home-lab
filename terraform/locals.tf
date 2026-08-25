locals {
  ssh_public_key = trimspace(file(pathexpand(var.ssh_public_key_path)))
  pool_name      = "${var.cluster_name}-k8s"
}
