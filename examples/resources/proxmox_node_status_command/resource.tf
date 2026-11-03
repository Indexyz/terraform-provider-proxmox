resource "proxmox_node_status_command" "reboot" {
  node         = "pve-1"
  command      = "reboot"
  wait_seconds = 120
  triggers     = { gen = "1" }
}
