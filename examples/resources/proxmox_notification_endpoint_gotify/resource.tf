resource "proxmox_notification_endpoint_gotify" "phone" {
  name   = "phone"
  server = "https://gotify.example.test"
  token  = var.gotify_token
}
