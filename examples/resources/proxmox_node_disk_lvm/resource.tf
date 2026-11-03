resource "proxmox_node_disk_lvm" "vg" {
  node   = "pve-1"
  name   = "vg1"
  device = "/dev/disk/by-id/ata-..."
}
