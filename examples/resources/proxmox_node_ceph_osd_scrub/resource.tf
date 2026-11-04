resource "proxmox_node_ceph_osd_scrub" "s" {
  node     = "pve-1"
  osdid    = 0
  triggers = { gen = "1" }
}
