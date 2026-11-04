resource "proxmox_guest_dbus_vmstate" "helper" {
  node     = "pve"
  vmid     = 100
  action   = "start"
  triggers = { revision = "1" }
}
