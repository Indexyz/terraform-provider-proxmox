resource "proxmox_node_ceph_init" "c" {
  node     = "pve-1"
  network  = "10.0.0.0/24"
  triggers = { gen = "1" }
}
