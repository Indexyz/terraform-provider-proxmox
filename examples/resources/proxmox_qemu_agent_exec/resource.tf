resource "proxmox_qemu_agent_exec" "uptime" {
  node     = "pve-1"
  vmid     = 300
  command  = ["/bin/sh", "-c", "uptime"]
  triggers = { gen = "1" }
}

output "stdout" {
  value = proxmox_qemu_agent_exec.uptime.stdout
}
