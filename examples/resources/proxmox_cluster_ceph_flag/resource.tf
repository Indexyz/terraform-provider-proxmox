resource "proxmox_cluster_ceph_flag" "noout" {
  flag    = "noout"
  enabled = true
}
