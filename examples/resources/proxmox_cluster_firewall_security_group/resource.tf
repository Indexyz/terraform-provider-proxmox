# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_cluster_firewall_security_group" "web" {
  name    = "web-servers"
  comment = "Rules shared by web guests"
}

resource "proxmox_firewall_rule" "web_https" {
  scope          = "security_group"
  security_group = proxmox_cluster_firewall_security_group.web.name
  type           = "in"
  action         = "ACCEPT"
  proto          = "tcp"
  dport          = "443"
}
