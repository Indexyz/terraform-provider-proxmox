resource "proxmox_node_time" "tz" {
  node     = "pve-1"
  timezone = "Europe/Berlin"
}
