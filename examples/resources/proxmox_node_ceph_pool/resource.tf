resource "proxmox_node_ceph_pool" "p" {
  node     = "pve-1"
  name     = "rbd"
  size     = 3
  min_size = 2
  pg_num   = 64
}
