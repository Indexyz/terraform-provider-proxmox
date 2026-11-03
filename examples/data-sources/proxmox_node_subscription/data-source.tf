data "proxmox_node_subscription" "sub" {
  node = "pve-1"
}

output "status" {
  value = data.proxmox_node_subscription.sub.status
}
