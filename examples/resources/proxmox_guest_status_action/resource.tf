resource "proxmox_guest_status_action" "shutdown" {
  type       = "qemu"
  node       = "pve"
  vmid       = 100
  action     = "shutdown"
  timeout    = 60
  force_stop = 1
  triggers   = { revision = "1" }
}
