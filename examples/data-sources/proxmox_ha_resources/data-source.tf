data "proxmox_ha_resources" "all" {}

output "ha_states" {
  value = { for resource in data.proxmox_ha_resources.all.resources : resource.sid => resource.state }
}
