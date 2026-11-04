data "proxmox_guest_proxy" "vnc" {
  type = "qemu"
  node = "pve-1"
  vmid = 300
  kind = "vncproxy"
}
