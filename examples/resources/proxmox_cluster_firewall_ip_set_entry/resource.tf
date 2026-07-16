# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_cluster_firewall_ip_set" "trusted" {
  name = "trusted"
}

resource "proxmox_cluster_firewall_ip_set_entry" "management" {
  ip_set  = proxmox_cluster_firewall_ip_set.trusted.name
  cidr    = "10.10.0.0/24"
  comment = "Management network"
}
