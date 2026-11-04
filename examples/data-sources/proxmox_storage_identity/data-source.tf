data "proxmox_storage_identity" "i" {
  node    = "pve-1"
  storage = "local"
}
