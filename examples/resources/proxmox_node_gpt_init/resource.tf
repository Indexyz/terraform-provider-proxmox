resource "proxmox_node_gpt_init" "disk" {
  node     = "pve-1"
  disk     = "/dev/disk/by-id/ata-..."
  triggers = { gen = "1" }
}
