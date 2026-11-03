data "proxmox_acl" "vm100" {
  path = "/vms/100"
  type = "user"
}
