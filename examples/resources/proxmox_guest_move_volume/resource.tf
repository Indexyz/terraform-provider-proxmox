resource "proxmox_guest_move_volume" "m" {
  type     = "lxc"
  node     = "pve-1"
  vmid     = 200
  volume   = "rootfs"
  storage  = "target"
  triggers = { gen = "1" }
}
