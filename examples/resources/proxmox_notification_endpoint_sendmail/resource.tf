resource "proxmox_notification_endpoint_sendmail" "ops" {
  name   = "ops-mail"
  mailto = ["ops@example.test"]
}
