data "proxmox_storage_rrd_image" "i" {
  node      = "pve-1"
  storage   = "local"
  timeframe = "hour"
  cf        = "AVERAGE"
  ds        = "used"
}
