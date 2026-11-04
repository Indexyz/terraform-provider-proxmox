resource "proxmox_qemu_cloudinit_pending" "ci" {
  node     = "pve-1"
  vmid     = 300
  values   = { ciuser = "admin" }
  triggers = { gen = "1" }
}
