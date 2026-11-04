data "proxmox_guest_rrd_image" "cpu" {
  type = "qemu"
  node = "pve"
  vmid = 100
  ds   = "cpu"
}
