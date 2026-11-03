data "proxmox_lxc_snapshots" "ct100" {
  node = "pve-1"
  vmid = 100
}
