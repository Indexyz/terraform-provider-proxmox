data "proxmox_qemu_agent" "facts" {
  node = "pve-1"
  vmid = 300
}
