# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Fabric nodes stay staged until proxmox_sdn_apply commits the SDN
# configuration. Interfaces are Proxmox property strings.
resource "proxmox_sdn_fabric_node" "node1" {
  fabric_id  = proxmox_sdn_fabric.ospf.fabric_id
  node_id    = "node1"
  protocol   = "ospf"
  ip         = "10.10.0.1"
  interfaces = ["name=eth1"]
}
