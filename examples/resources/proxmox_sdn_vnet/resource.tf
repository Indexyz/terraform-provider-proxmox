# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# VNets stay staged until proxmox_sdn_apply commits the configuration.
# Credentials need SDN.Allocate on the parent zone.

# A plain VNet inside the lab zone.
resource "proxmox_sdn_vnet" "app" {
  vnet  = "vnapp"
  zone  = proxmox_sdn_zone.lab.zone
  alias = "Application network"
}

# A VLAN VNet in a VLAN zone. `tag` is the VLAN id handed to the zone bridge.
# A VNet with subnets must not be `vlanaware`.
resource "proxmox_sdn_vnet" "vlan100" {
  vnet          = "vn100"
  zone          = proxmox_sdn_zone.vlan.zone
  tag           = 100
  isolate_ports = true
  alias         = "Tenant 100"
}
