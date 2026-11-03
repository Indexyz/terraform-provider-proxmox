data "proxmox_qemu_snapshots" "vm100" {
  node = "pve-1"
  vmid = 100
}
