resource "proxmox_node_disk_lvmthin" "thin" {
  node   = "pve-1"
  name   = "thin1"
  device = "/dev/disk/by-id/ata-..."
}
