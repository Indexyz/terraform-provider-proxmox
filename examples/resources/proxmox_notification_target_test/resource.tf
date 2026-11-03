resource "proxmox_notification_target_test" "check" {
  name     = "mail-to-root"
  triggers = { gen = "1" }
}
