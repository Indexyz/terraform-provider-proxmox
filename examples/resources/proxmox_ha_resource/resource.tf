# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# The guest must already exist. Provider credentials need Sys.Audit and
# Sys.Console on / for the complete Terraform lifecycle.
resource "proxmox_ha_resource" "database" {
  resource_id    = "vm:120"
  state          = "started"
  comment        = "Database guest managed by Terraform"
  failback       = true
  auto_rebalance = false
  max_restart    = 2
  max_relocate   = 2
}
