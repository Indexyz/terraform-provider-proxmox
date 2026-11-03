resource "proxmox_user_tfa_unlock" "root" {
  user_id  = "root@pam"
  triggers = { reason = "manual" }
}
