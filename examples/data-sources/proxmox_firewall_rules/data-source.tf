# Cluster ruleset (the default scope).
data "proxmox_firewall_rules" "cluster" {}

# Rules of one guest.
data "proxmox_firewall_rules" "vm100" {
  scope      = "guest"
  node       = "pve-1"
  guest_type = "qemu"
  vmid       = 100
}
