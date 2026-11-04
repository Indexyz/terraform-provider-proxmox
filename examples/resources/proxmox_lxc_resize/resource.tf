resource "proxmox_lxc_resize" "rootfs" {
  node     = "pve"
  vmid     = 101
  disk     = "rootfs"
  size     = "+2G"
  triggers = { revision = "1" }
}
