resource "proxmox_notification_matcher" "errors" {
  name           = "errors-to-ops"
  target         = [proxmox_notification_endpoint_sendmail.ops.name]
  match_severity = ["error"]
}
