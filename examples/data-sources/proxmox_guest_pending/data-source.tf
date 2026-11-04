data "proxmox_guest_pending" "qemu" {
  type = "qemu"
  node = "pve"
  vmid = 100
}
