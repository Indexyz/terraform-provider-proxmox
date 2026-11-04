resource "proxmox_node_wakeonlan" "w" {
  node     = "pve-1"
  triggers = { gen = "1" }
}
