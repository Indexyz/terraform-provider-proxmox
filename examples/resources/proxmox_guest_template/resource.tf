resource "proxmox_guest_template" "base" {
  type     = "qemu"
  node     = "pve"
  vmid     = 100
  disks    = ["scsi0"]
  triggers = { revision = "1" }
}
