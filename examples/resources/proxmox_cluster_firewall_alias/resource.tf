# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_cluster_firewall_alias" "monitoring" {
  name    = "monitoring"
  cidr    = "10.20.0.10"
  comment = "Monitoring server"
}
