resource "proxmox_node_ceph_mgr" "mgr" {
  node     = "pve-1"
  name     = "mgr1"
  triggers = { gen = "1" }
}
