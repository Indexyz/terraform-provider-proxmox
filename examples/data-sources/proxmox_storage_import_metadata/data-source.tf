data "proxmox_storage_import_metadata" "m" {
  node    = "pve-1"
  storage = "local"
  volume  = "local:import/disk.img"
}
