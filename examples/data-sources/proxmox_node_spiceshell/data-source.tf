data "proxmox_node_spiceshell" "s" {
  node  = "pve-1"
  proxy = "https://pve-1:8006"
}
