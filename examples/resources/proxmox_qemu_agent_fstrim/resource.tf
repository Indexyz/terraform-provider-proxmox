resource "proxmox_qemu_agent_fstrim" "f" {
  node     = "pve-1"
  vmid     = 300
  triggers = { gen = "1" }
}
