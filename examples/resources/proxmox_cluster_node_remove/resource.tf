resource "proxmox_cluster_node_remove" "old" {
  node     = "pve-old"
  triggers = { gen = "1" }
}
