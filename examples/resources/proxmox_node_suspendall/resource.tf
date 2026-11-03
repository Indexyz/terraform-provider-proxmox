resource "proxmox_node_suspendall" "all" {
  node     = "pve-1"
  triggers = { gen = "1" }
}
