resource "proxmox_qemu_monitor" "m" {
  node     = "pve-1"
  vmid     = 300
  command  = "info status"
  triggers = { gen = "1" }
}
