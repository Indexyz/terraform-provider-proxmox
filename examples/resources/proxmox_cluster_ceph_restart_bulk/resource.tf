resource "proxmox_cluster_ceph_restart_bulk" "outdated" {
  service_type  = "osd"
  only_outdated = true
  triggers      = { run = "2026-11-04" }
}
