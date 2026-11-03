resource "proxmox_node_execute" "batch" {
  node = "pve-1"
  calls = [
    { method = "GET", path = "status" },
    { method = "POST", path = "qemu/300/status/start" },
  ]
  triggers = { gen = "1" }
}

output "results" {
  value = jsondecode(proxmox_node_execute.batch.results)
}
