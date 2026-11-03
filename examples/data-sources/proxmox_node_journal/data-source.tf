data "proxmox_node_journal" "recent" {
  node  = "pve-1"
  limit = 20
}
