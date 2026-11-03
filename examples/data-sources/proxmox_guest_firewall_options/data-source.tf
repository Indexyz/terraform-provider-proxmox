data "proxmox_guest_firewall_options" "vm100" {
  node       = "pve-1"
  guest_type = "qemu"
  vm_id      = 100
}
