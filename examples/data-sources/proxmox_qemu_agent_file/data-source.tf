data "proxmox_qemu_agent_file" "f" {
  node = "pve-1"
  vmid = 300
  file = "/etc/hostname"
}
