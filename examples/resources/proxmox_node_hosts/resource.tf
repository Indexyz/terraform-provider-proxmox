resource "proxmox_node_hosts" "h" {
  node = "pve-1"
  data = "127.0.0.1 localhost\n"
}
