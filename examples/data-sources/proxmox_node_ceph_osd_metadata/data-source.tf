data "proxmox_node_ceph_osd_metadata" "m" {
  node  = "pve-1"
  osdid = 0
}
