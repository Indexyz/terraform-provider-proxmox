data "proxmox_node_rrd_image" "i" {
  node      = "pve-1"
  timeframe = "hour"
  cf        = "AVERAGE"
  ds        = "cpu"
}
