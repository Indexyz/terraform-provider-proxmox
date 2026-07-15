# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_cluster_firewall_options" "cluster" {
  enable         = true
  ebtables       = true
  policy_in      = "DROP"
  policy_out     = "ACCEPT"
  policy_forward = "DROP"
  log_ratelimit  = "enable=1,burst=5,rate=1/second"
}
