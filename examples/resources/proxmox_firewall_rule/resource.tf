# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_firewall_rule" "allow_https" {
  type   = "in"
  action = "ACCEPT"
  source = "10.0.0.0/8"
  proto  = "tcp"
  dport  = "443"
}

# Rules can also be scoped to an SDN VNet. The VNet firewall must be enabled
# through proxmox_sdn_vnet_firewall_options first.
resource "proxmox_firewall_rule" "vnet_forward" {
  scope  = "vnet"
  vnet   = proxmox_sdn_vnet.app.vnet
  type   = "forward"
  action = "ACCEPT"
  proto  = "tcp"
  dport  = "53"
}
