resource "proxmox_node_ceph_mds" "mds" {
  node     = "pve-1"
  name     = "mds1"
  triggers = { gen = "1" }
}
