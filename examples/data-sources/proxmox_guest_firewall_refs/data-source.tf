data "proxmox_guest_firewall_refs" "qemu" {
  type = "qemu"
  node = "pve"
  vmid = 100
}
