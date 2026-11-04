resource "proxmox_guest_unlink" "detach" {
  node     = "pve"
  vmid     = 100
  idlist   = ["scsi1"]
  triggers = { revision = "1" }
}
