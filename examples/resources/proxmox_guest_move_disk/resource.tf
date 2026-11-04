resource "proxmox_guest_move_disk" "m" {
  node     = "pve-1"
  vmid     = 300
  disk     = "scsi0"
  storage  = "target"
  delete   = true
  triggers = { gen = "1" }
}
