data "proxmox_guest_vnc_websocket" "qemu" {
  type       = "qemu"
  node       = "pve"
  vmid       = 100
  vnc_ticket = "PVE:root@pam:VNC_TICKET"
  port       = "5900"
}
