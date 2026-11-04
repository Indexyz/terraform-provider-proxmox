data "proxmox_guest_rrddata" "cpu" {
  type = "qemu"
  node = "pve"
  vmid = 100
}
