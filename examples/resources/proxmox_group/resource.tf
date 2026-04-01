resource "proxmox_group" "developers" {
  group_id = "developers"
  comment  = "Managed by Terraform"
}
