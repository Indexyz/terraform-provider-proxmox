resource "proxmox_pool" "platform" {
  pool_id     = "platform"
  comment     = "Managed by Terraform"
  vm_ids      = [101, 102]
  storage_ids = ["local-lvm"]
}
