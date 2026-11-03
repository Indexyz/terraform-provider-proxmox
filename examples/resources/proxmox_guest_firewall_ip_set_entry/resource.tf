resource "proxmox_guest_firewall_ip_set_entry" "office" {
  node       = "pve-1"
  guest_type = "qemu"
  vmid       = 100
  ip_set     = proxmox_guest_firewall_ip_set.office.name
  cidr       = "192.0.2.0/24"
}
