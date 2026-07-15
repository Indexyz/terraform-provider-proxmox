# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_backup_job" "nightly" {
  job_id            = "nightly-production"
  all               = true
  exclude_vm_ids    = "900"
  enabled           = true
  schedule          = "02:00"
  storage           = "backup"
  mode              = "snapshot"
  compression       = "zstd"
  prune_backups     = "keep-daily=7,keep-monthly=6,keep-weekly=4"
  repeat_missed     = true
  notes_template    = "{{guestname}} ({{vmid}})"
  notification_mode = "notification-system"
}
