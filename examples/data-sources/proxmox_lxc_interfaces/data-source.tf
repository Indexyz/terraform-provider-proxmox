data "proxmox_lxc_interfaces" "i" {
  node = "pve-1"
  vmid = 200
}
