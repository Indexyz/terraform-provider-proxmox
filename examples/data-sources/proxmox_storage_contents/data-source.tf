data "proxmox_storage_contents" "iso" {
  node    = "pve-1"
  storage = "local"
  content = "iso"
}
