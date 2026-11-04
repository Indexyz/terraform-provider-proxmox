data "proxmox_qemu_cloudinit_dump" "c" {
  node = "pve-1"
  vmid = 300
  type = "user"
}
