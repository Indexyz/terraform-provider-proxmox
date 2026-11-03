# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Discards every staged SDN change by restoring the running snapshot.
# Use it as an explicit "throw the staging away" step; destroying the resource
# does not re-apply anything. Credentials need SDN.Allocate on /sdn.
resource "proxmox_sdn_rollback" "discard" {
  triggers = {
    revision = "2026-11-03"
  }
}
