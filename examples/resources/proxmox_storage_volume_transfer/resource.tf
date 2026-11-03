resource "proxmox_storage_volume_transfer" "copy" {
  node     = "pve-1"
  storage  = "local"
  volume   = "local:300/vm-300-disk-0.raw"
  target   = "local:300/vm-300-disk-0-copy.raw"
  triggers = { gen = "1" }
}
