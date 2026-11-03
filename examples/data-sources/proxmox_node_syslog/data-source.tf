data "proxmox_node_syslog" "recent" {
  node  = "pve-1"
  limit = 20
}
