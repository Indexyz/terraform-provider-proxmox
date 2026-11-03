resource "proxmox_guest_firewall_ip_set" "office" {
  node       = "pve-1"
  guest_type = "qemu"
  vmid       = 100
  name       = "office"
  comment    = "office networks"
}
