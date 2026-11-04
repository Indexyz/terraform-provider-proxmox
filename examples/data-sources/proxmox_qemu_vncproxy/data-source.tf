data "proxmox_qemu_vncproxy" "v" {
  node = "pve-1"
  vmid = 300
}
