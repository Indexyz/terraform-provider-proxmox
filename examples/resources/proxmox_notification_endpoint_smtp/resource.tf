resource "proxmox_notification_endpoint_smtp" "relay" {
  name         = "relay"
  server       = "smtp.example.test"
  port         = 587
  from_address = "pve@example.test"
  mode         = "starttls"
  mailto       = ["ops@example.test"]
  username     = "pve"
  password     = var.smtp_password
}
