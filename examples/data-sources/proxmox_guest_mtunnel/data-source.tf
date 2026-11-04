data "proxmox_guest_mtunnel" "t" {
  type = "qemu"
  node = "pve-1"
  vmid = 300
}
