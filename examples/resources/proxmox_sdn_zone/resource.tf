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

# A VXLAN zone tunnelling to the node IPs, with an explicit UDP port.
resource "proxmox_sdn_zone" "vx" {
  zone       = "vxz1"
  type       = "vxlan"
  peers      = ["10.0.0.1", "10.0.0.2"]
  vxlan_port = 4789
}

# An EVPN zone with its required controller and VRF VNI.
resource "proxmox_sdn_zone" "evpn" {
  zone                       = "evz1"
  type                       = "evpn"
  controller                 = "evpn-controller"
  vrf_vxlan                  = 10000
  exitnodes                  = ["node1"]
  exitnodes_local_routing    = true
  advertise_subnets          = true
  disable_arp_nd_suppression = false
  mac                        = "AA:BB:CC:DD:EE:FF"
}

# A QinQ zone needs the outer service VLAN tag and a bridge.
resource "proxmox_sdn_zone" "qinq" {
  zone          = "qz1"
  type          = "qinq"
  bridge        = "vmbr1"
  tag           = 100
  vlan_protocol = "802.1ad"
}

# A faucet zone needs a controller and the dataplane id.
resource "proxmox_sdn_zone" "faucet" {
  zone       = "fz1"
  type       = "faucet"
  controller = "faucet-controller"
  dp_id      = 1
}
