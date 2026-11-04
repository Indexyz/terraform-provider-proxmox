data "proxmox_qemu_agent_fsfreeze_status" "f" {
  node = "pve-1"
  vmid = 300
}
