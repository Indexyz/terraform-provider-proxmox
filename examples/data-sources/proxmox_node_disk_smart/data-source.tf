data "proxmox_node_disk_smart" "disk" {
  node = "pve-1"
  disk = "/dev/disk/by-id/ata-..."
}
