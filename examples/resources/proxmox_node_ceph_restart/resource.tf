resource "proxmox_node_ceph_restart" "r" {
  node    = "pve-1"
  service = "osd.0"
}
