resource "proxmox_node_ceph_start" "osd" {
  node     = "pve"
  service  = "osd.0"
  triggers = { revision = "1" }
}
