data "proxmox_node_ceph_osd_lv_info" "l" {
  node  = "pve-1"
  osdid = 0
  type  = "block"
}
