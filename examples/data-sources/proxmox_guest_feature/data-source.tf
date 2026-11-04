data "proxmox_guest_feature" "snapshot" {
  type    = "qemu"
  node    = "pve"
  vmid    = 100
  feature = "snapshot"
}
