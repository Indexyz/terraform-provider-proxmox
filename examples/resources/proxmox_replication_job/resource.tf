# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_replication_job" "database" {
  job_id   = "101-0"
  target   = "pve-2"
  schedule = "*/15"
  rate     = 50
  comment  = "Replicate database guest"
}
