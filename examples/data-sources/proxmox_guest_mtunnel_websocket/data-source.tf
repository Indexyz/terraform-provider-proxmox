data "proxmox_guest_mtunnel_websocket" "w" {
  type   = "qemu"
  node   = "pve-1"
  vmid   = 300
  socket = "/run/qemu-server/300.mtunnel"
  ticket = "PVETUNNEL:..."
}
