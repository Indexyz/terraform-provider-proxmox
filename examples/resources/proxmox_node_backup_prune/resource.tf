resource "proxmox_node_backup_prune" "keep_last" {
  node          = "pve-1"
  storage       = "backup"
  vmid          = 300
  prune_backups = "keep-last=3"
  triggers      = { gen = "1" }
}
