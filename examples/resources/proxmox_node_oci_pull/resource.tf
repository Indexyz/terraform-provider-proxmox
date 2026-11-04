resource "proxmox_node_oci_pull" "img" {
  node      = "pve-1"
  storage   = "local"
  reference = "docker.io/library/alpine:latest"
  triggers  = { gen = "1" }
}
