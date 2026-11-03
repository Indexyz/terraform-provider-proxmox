resource "proxmox_guest_migration" "move" {
  node     = "pve-1"
  vmid     = 300
  target   = "pve-2"
  online   = true
  triggers = { gen = "1" }
}
