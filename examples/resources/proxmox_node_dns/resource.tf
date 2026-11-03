resource "proxmox_node_dns" "dns" {
  node   = "pve-1"
  dns1   = "10.0.0.53"
  search = "example.com"
}
