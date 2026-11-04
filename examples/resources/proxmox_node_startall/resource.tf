resource "proxmox_node_startall" "s" {
  node     = "pve-1"
  force    = true
  triggers = { gen = "1" }
}
