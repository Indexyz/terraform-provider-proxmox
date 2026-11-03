data "proxmox_qemu_rrddata" "cpu" {
  node      = "pve-1"
  vmid      = 300
  timeframe = "day"
}
