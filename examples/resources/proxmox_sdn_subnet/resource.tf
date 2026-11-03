# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Subnets belong to a VNet and only take effect after proxmox_sdn_apply.
# The parent VNet must not be `vlanaware`.
# Credentials need SDN.Allocate on the parent VNet.

resource "proxmox_sdn_subnet" "app" {
  vnet    = proxmox_sdn_vnet.vlan100.vnet
  cidr    = "10.100.0.0/24"
  gateway = "10.100.0.1"
  snat    = true

  dhcp_range = [
    {
      start_address = "10.100.0.10"
      end_address   = "10.100.0.50"
    },
  ]
}

# IPv6 subnet with a DNS zone prefix instead of DHCP.
resource "proxmox_sdn_subnet" "app_v6" {
  vnet          = proxmox_sdn_vnet.vlan100.vnet
  cidr          = "fd00:100::/64"
  dnszoneprefix = "app"
}
