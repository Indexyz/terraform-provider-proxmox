# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_firewall_rule" "allow_https" {
  type   = "in"
  action = "ACCEPT"
  source = "10.0.0.0/8"
  proto  = "tcp"
  dport  = "443"
}
