# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Fabrics stay staged until proxmox_sdn_apply commits the SDN configuration.
# Provider credentials need SDN.Allocate on /sdn/fabrics.

resource "proxmox_sdn_prefix_list" "underlay" {
  prefix_list_id = "underlay"
  entries        = ["action=permit,prefix=10.10.0.0/16"]
}

# An OSPF fabric using that prefix list as route filter.
resource "proxmox_sdn_fabric" "ospf" {
  fabric_id    = "fab1"
  protocol     = "ospf"
  ip_prefix    = "10.10.0.0/24"
  area         = "0.0.0.1"
  route_filter = proxmox_sdn_prefix_list.underlay.prefix_list_id
}
