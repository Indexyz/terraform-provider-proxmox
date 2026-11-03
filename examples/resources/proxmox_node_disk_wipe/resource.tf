resource "proxmox_node_disk_wipe" "disk" {
  node     = "pve-1"
  disk     = "/dev/disk/by-id/ata-..."
  triggers = { gen = "1" }
}
