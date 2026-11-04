data "proxmox_node_tasks" "t" {
  node  = "pve-1"
  limit = 20
}
