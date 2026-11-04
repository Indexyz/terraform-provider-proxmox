data "proxmox_storage_rrddata" "d" {
  node      = "pve-1"
  storage   = "local"
  timeframe = "hour"
}
