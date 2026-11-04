resource "proxmox_storage_upload" "seed" {
  node     = "pve"
  storage  = "local"
  content  = "iso"
  source   = "${path.module}/seed.iso"
  triggers = { revision = "1" }
}
