resource "proxmox_node_ceph_monitor" "m" {
  node  = "pve-1"
  monid = "pve-1"
}
