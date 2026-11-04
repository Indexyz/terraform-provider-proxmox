data "proxmox_access_permissions" "root" {
  path   = "/"
  userid = "root@pam"
}
