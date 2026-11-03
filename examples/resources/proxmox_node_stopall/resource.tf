resource "proxmox_node_stopall" "all" {
  node       = "pve-1"
  force_stop = true
  triggers   = { gen = "1" }
}
