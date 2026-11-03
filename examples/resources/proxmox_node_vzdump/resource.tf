resource "proxmox_node_vzdump" "nightly" {
  node     = "pve-1"
  vmid     = 300
  storage  = "backup"
  mode     = "snapshot"
  compress = "zstd"
  triggers = { gen = "1" }
}
