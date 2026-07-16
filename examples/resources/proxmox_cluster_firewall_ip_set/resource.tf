# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_cluster_firewall_ip_set" "trusted" {
  name    = "trusted"
  comment = "Trusted management networks"
}
