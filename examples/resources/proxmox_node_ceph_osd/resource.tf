resource "proxmox_node_ceph_osd" "d" {
  node = "pve-1"
  dev  = "/dev/sdb"
}
