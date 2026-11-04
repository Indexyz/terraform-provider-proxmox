resource "proxmox_qemu_sendkey" "k" {
  node     = "pve-1"
  vmid     = 300
  key      = "a"
  triggers = { gen = "1" }
}
