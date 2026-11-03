resource "proxmox_cluster_bulk_action" "start_all" {
  action   = "start"
  vms      = [300, 301]
  triggers = { gen = "1" }
}
