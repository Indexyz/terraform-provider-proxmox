resource "proxmox_node_service" "spice" {
  node    = "pve-1"
  service = "spiceproxy"
  state   = "running"
}
