resource "proxmox_node_disk_zfs" "tank" {
  node        = "pve-1"
  name        = "tank"
  devices     = ["/dev/disk/by-id/ata-..."]
  raid_level  = "mirror"
  compression = "lz4"
}
