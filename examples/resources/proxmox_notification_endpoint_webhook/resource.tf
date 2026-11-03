# Header values are base64; `secret` redacts them in the Proxmox UI.
resource "proxmox_notification_endpoint_webhook" "hook" {
  name   = "hook"
  url    = "https://hook.example.test/notify"
  method = "post"
  header = ["name=token,value=YWJj"]
  secret = ["name=token,value=YWJj"]
  body   = "{}"
}
