resource "proxmox_guest_cloudinit" "regen" {
  node     = "pve"
  vmid     = 100
  triggers = { revision = "1" }
}
