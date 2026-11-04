resource "proxmox_node_ceph_fs" "fs" {
  node     = "pve-1"
  name     = "cephfs"
  triggers = { gen = "1" }
}
