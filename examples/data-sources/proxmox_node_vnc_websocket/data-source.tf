data "proxmox_node_vnc_websocket" "shell" {
  node       = "pve"
  vnc_ticket = "PVE:root@pam:VNC_TICKET"
  port       = "5900"
}
