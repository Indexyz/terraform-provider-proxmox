data "proxmox_node_ceph_log" "l" {
  node  = "pve-1"
  limit = 20
}
