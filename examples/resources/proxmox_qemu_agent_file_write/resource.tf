resource "proxmox_qemu_agent_file_write" "f" {
  node     = "pve-1"
  vmid     = 300
  file     = "/tmp/hello"
  content  = "hello"
  triggers = { gen = "1" }
}
