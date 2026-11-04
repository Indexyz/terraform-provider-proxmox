resource "proxmox_ha_resource_migrate" "move" {
  sid      = "vm:100"
  action   = "migrate"
  node     = "pve2"
  triggers = { revision = "1" }
}
