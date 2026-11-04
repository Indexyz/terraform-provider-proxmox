data "proxmox_access_ticket" "root" {
  username = "root@pam"
  password = var.proxmox_password
}
