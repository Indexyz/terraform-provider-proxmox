resource "proxmox_node_apt_update" "daily" {
  node     = "pve-1"
  triggers = { gen = "1" }
}
