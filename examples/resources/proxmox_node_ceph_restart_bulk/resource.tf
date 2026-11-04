resource "proxmox_node_ceph_restart_bulk" "r" {
  node         = "pve-1"
  service_type = "osd"
}
