data "proxmox_schedule_analyze" "a" {
  schedule   = "*-*-* 03:00"
  iterations = 5
}
