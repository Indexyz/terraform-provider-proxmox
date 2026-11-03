data "proxmox_node_rrddata" "cpu" {
  node      = "pve-1"
  timeframe = "hour"
  cf        = "AVERAGE"
}

output "points" {
  value = jsondecode(data.proxmox_node_rrddata.cpu.data)
}
