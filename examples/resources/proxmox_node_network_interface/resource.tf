# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Interfaces are only staged in /etc/network/interfaces.new. Nothing is
# activated until proxmox_node_network_apply for the node runs.
# Provider credentials need Sys.Modify on /nodes/{node}.

# A VLAN-aware bridge for guests, with one uplink port.
resource "proxmox_node_network_interface" "vmbr1" {
  node              = "node1"
  iface             = "vmbr1"
  type              = "bridge"
  address           = "10.0.1.2"
  netmask           = "255.255.255.0"
  bridge_ports      = ["eth1"]
  bridge_vlan_aware = true
  bridge_vids       = "2 4 100-200"
  comments          = "Guest bridge managed by Terraform"
  autostart         = true
}

# A VLAN on top of the uplink, with jumbo frames.
resource "proxmox_node_network_interface" "vlan100" {
  node            = "node1"
  iface           = "eth1.100"
  type            = "vlan"
  vlan_id         = 100
  vlan_raw_device = "eth1"
  mtu             = 9000
}

# Open vSwitch bridge. `ovs_ports` is maintained by Proxmox from the port
# interfaces, so it is computed-only.
resource "proxmox_node_network_interface" "ovsbr0" {
  node  = "node1"
  iface = "ovsbr0"
  type  = "OVSBridge"
  mtu   = 9000
}

# Internal OVS port with a VLAN tag; ovs_bridge is required for OVSIntPort and
# OVSBond because Proxmox cannot place them otherwise.
# An existing physical NIC becomes an OVS port by declaring it as one;
# adoption keeps its unmanaged settings.
#
resource "proxmox_node_network_interface" "tenant100" {
  node        = "node1"
  iface       = "ovs100"
  type        = "OVSIntPort"
  ovs_bridge  = proxmox_node_network_interface.ovsbr0.iface
  ovs_tag     = 100
  ovs_options = "trunks=100,200"
}

# An alias interface adds an extra address to a parent bridge. The name needs
# the `:<n>` suffix; Proxmox only keeps the alias while the parent exists.
resource "proxmox_node_network_interface" "vmbr1_alias" {
  node    = "node1"
  iface   = "vmbr1:1"
  type    = "alias"
  address = "10.0.0.2"
  netmask = "24"
}
