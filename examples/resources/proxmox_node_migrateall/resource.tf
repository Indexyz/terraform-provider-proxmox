resource "proxmox_node_migrateall" "m" {
  node     = "pve-1"
  target   = "pve-2"
  triggers = { gen = "1" }
}
