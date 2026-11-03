# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Controllers stay staged until proxmox_sdn_apply commits the SDN
# configuration. EVPN zones reference them through `controller`.
# Provider credentials need SDN.Allocate on /sdn/controllers.

resource "proxmox_sdn_controller" "bgp" {
  controller = "bgp1"
  type       = "bgp"
  node       = "node1"
  asn        = 65000
  peers      = ["10.0.0.2", "10.0.0.3"]
  loopback   = "lo"
}

resource "proxmox_sdn_controller" "evpn" {
  controller = "evpn1"
  type       = "evpn"
  asn        = 65000
  peers      = ["10.0.0.2"]
  fabric     = "fabric1"
}

resource "proxmox_sdn_controller" "isis" {
  controller  = "isis1"
  type        = "isis"
  node        = "node1"
  isis_domain = "lab"
  isis_net    = "49.0001.1111.1111.1111.00"
  isis_ifaces = ["eth1", "eth2"]
}
