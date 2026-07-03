# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_node_firewall_options" "pve_1" {
  node                = "pve-1"
  enable              = true
  log_level_in        = "info"
  protection_synflood = true
  nosmurfs            = true
}
