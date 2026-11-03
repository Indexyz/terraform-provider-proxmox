resource "proxmox_guest_firewall_alias" "mail" {
  node       = "pve-1"
  guest_type = "qemu"
  vmid       = 100
  name       = "mail"
  cidr       = "192.0.2.5"
  comment    = "mail relay"
}
