resource "proxmox_node_disk_directory" "iso" {
  node       = "pve-1"
  name       = "iso"
  device     = "/dev/disk/by-id/ata-..."
  filesystem = "xfs"
}
