# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Zones stay staged until proxmox_sdn_apply commits the configuration.
# Provider credentials need SDN.Allocate on /sdn and /sdn/zones.

# A plain zone without type-specific fields.
resource "proxmox_sdn_zone" "lab" {
  zone  = "lab"
  type  = "simple"
  nodes = ["node1", "node2"]
}

# A VLAN zone bound to the VLAN-aware bridge whose VLANs it manages.
# `dns` and `ipam` reference SDN DNS and IPAM object ids.
resource "proxmox_sdn_zone" "vlan" {
  zone   = "vz100"
  type   = "vlan"
  nodes  = ["node1", "node2"]
  bridge = "vmbr1"
  mtu    = 9000

  dns     = "dns1"
  dnszone = "example.com"
  ipam    = "pve"
  dhcp    = "dnsmasq"
}
